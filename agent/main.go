package main

import (
	"context"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const ipcAddress = "127.0.0.1:9875"

type service struct {
	brain  *brain
	cli    *nativeCLI
	tts    *speaker
	budget *dailyBudget
	pulse  ringIndicator
	arb    *arbiter
	wake   []string
	dry    bool
	logf   func(string, ...any)

	mu          sync.Mutex
	busy        bool
	followUntil time.Time
	pending     *pendingTurn
	seen        map[string]bool
}

// pendingTurn ties one recognised phrase to the cloud answer for the same
// request. Only the station that the cloud actually replies to is allowed to
// speak, which is how the stock multi-room setup picks the nearest station.
type pendingTurn struct {
	requestID string
	wakeOnly  bool
	won       chan struct{}
	once      sync.Once
}

func main() {
	configPath := flag.String("config", "/data/jarvis-native/config.json", "cloud configuration")
	logPath := flag.String("log", "/data/jarvis-native/log/agent.log", "service log (content redacted by default)")
	listen := flag.Bool("listen", false, "read-only event log, no answers")
	dry := flag.Bool("dry-run", false, "full pipeline without speaking")
	dump := flag.String("dump", "", "write raw IPC frames to this file (diagnostics)")
	say := flag.String("say", "", "speak the given text through the configured voice and exit")
	ask := flag.String("ask", "", "run one full answer for the given question and exit")
	seconds := flag.Int("seconds", 60, "observation window for -listen")
	led := flag.Int("led", 0, "pulse the status ring for N seconds and exit (diagnostics)")
	flag.Parse()

	logf, closeLog, err := openLog(*logPath)
	if err != nil {
		fmt.Println("log:", err)
		os.Exit(1)
	}
	defer closeLog()

	if *listen {
		if err := observe(*seconds, logf); err != nil {
			logf("observe: %v", err)
		}
		return
	}
	if *dump != "" {
		if err := dumpStream(*dump, *seconds, logf); err != nil {
			logf("dump: %v", err)
		}
		return
	}
	if *led > 0 {
		p, err := openIndicator()
		if err != nil {
			logf("led: %v", err)
			os.Exit(1)
		}
		p.pulseFor(time.Duration(*led)*time.Second, 140)
		time.Sleep(time.Duration(*led)*time.Second + 300*time.Millisecond)
		p.close()
		logf("led test finished")
		return
	}

	br, err := loadBrain(*configPath, logf)
	if err != nil {
		logf("config: %v", err)
		os.Exit(1)
	}
	// Warm the weather cache in the background so the first question about
	// the weather does not wait for the network.
	if br.weather != nil {
		br.weather.current()
	}
	cli := &nativeCLI{}
	if *say != "" {
		sp := newSpeaker(br.cfg.TTSProvider, cli, logf)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		if err := sp.speak(ctx, *say); err != nil {
			logf("say: %v", err)
			cli.close()
			os.Exit(1)
		}
		cli.close()
		return
	}
	if *ask != "" {
		sp := newSpeaker(br.cfg.TTSProvider, cli, logf)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		start := time.Now()
		answer, err := br.ask(ctx, *ask)
		if err != nil {
			logf("ask: %v", err)
			cli.close()
			os.Exit(1)
		}
		logf("ask answer in %s: %q", time.Since(start).Round(time.Millisecond), trimForLog(answer))
		if answer == "" || *dry {
			cli.close()
			return
		}
		if err := sp.speak(ctx, answer); err != nil {
			logf("speak: %v", err)
			cli.close()
			os.Exit(1)
		}
		cli.close()
		return
	}
	budget, err := openBudget(filepath.Join(filepath.Dir(*configPath), "usage-native.json"), br.cfg.DailyTurnLimit, br.cfg.TimeOffsetHours)
	if err != nil {
		logf("budget: %v", err)
		os.Exit(1)
	}
	pulse, perr := openIndicator()
	if perr != nil {
		logf("indicator unavailable: %v", perr)
	}

	svc := &service{
		brain: br, cli: cli, tts: newSpeaker(br.cfg.TTSProvider, cli, logf),
		budget: budget, pulse: pulse, wake: wakeWords(br.cfg.WakeWords),
		arb: newArbiter(br.cfg.StationID, br.cfg.StationPriority, br.cfg.Peers, br.cfg.ArbPort, logf),
		dry: *dry, logf: logf, seen: map[string]bool{},
	}
	logf("station %q priority=%d peers=%v", br.cfg.StationID, br.cfg.StationPriority, br.cfg.Peers)
	pidPath := "/var/run/jarvis-native.pid"
	if *dry {
		pidPath = ""
	}
	if pidPath != "" {
		os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
		defer os.Remove(pidPath)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logf("starting native bridge; model=%s provider=%s wake=%q dry=%v", br.cfg.ChatModel, br.cfg.Provider, svc.wake, *dry)

	// Keep the CLI warm: its start-up takes about two seconds.
	if !*dry {
		go func() {
			if err := svc.cli.ensure(); err != nil {
				logf("native CLI warm-up: %v", err)
			} else {
				logf("native CLI ready: %s", svc.cli.status())
			}
		}()
	}
	svc.run(ctx)
	svc.cli.close()
	if pulse != nil {
		pulse.close()
	}
	logf("stopped")
}

func (s *service) run(ctx context.Context) {
	for ctx.Err() == nil {
		conn, err := net.DialTimeout("tcp", ipcAddress, 3*time.Second)
		if err != nil {
			s.logf("ipc connect: %v; retrying", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
			continue
		}
		s.logf("ipc connected to %s", ipcAddress)
		s.serve(ctx, conn)
		conn.Close()
		if ctx.Err() == nil {
			s.logf("ipc stream closed; reconnecting")
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}
}

func (s *service) serve(ctx context.Context, conn net.Conn) {
	connected := time.Now()
	for ctx.Err() == nil {
		payload, err := readFrame(conn)
		if err != nil {
			return
		}
		state, ok := decodeAliceState(payload)
		if !ok {
			continue
		}
		s.notifyResponse(state)
		if !state.Final || state.Phrase == "" {
			continue
		}
		// A fresh subscriber receives the last cached state; never replay it.
		if time.Since(connected) < 800*time.Millisecond {
			s.mark(state.RequestID)
			continue
		}
		if s.mark(state.RequestID) {
			continue
		}
		s.onFinal(state)
	}
}

// cloudAnswered reports whether the cloud sent anything at all for a request.
// In a multi-room setup only the station the cloud picked receives a reply;
// the others just fall back to idle, which makes this the arbitration signal.
func cloudAnswered(state *aliceState) bool {
	return state.TTSText != "" || len(state.Response) > 0
}

// notifyResponse releases the pending turn as soon as the cloud answers the
// same request, and silences Alice so she does not speak over the answer.
func (s *service) notifyResponse(state *aliceState) {
	if !cloudAnswered(state) {
		return
	}
	s.mu.Lock()
	p := s.pending
	s.mu.Unlock()
	if p == nil || p.requestID != state.RequestID {
		return
	}
	p.once.Do(func() {
		close(p.won)
		// A wake-only turn already cancelled Alice immediately; a second
		// cancel would kill the follow-up dialog it has just started.
		if p.wakeOnly {
			return
		}
		go func() {
			if err := s.cli.cancel(); err != nil {
				s.logf("cancel: %v", err)
			}
		}()
	})
}

func (s *service) clearFollowUp() {
	s.mu.Lock()
	s.followUntil = time.Time{}
	s.mu.Unlock()
}

func (s *service) followUpActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.followUntil.IsZero() && time.Now().Before(s.followUntil)
}

func (s *service) armFollowUp(d time.Duration) {
	s.mu.Lock()
	s.followUntil = time.Now().Add(d)
	s.mu.Unlock()
}

func (s *service) mark(id string) bool {
	if id == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seen[id] {
		return true
	}
	if len(s.seen) > 256 {
		s.seen = map[string]bool{}
	}
	s.seen[id] = true
	return false
}

func (s *service) onFinal(state *aliceState) {
	question, ok := splitWake(state.Phrase, s.wake)
	viaFollowUp := false
	if ok {
		s.clearFollowUp()
	} else if s.followUpActive() {
		s.clearFollowUp()
		question, ok, viaFollowUp = stripAddress(normalizeSpeech(state.Phrase)), true, true
	}
	if !ok {
		if looksLikeWake(state.Phrase, s.wake) {
			s.logf("near-miss wake: %q", trimForLog(state.Phrase))
		}
		return
	}
	if viaFollowUp {
		s.logf("follow-up: %q", trimForLog(state.Phrase))
	} else {
		s.logf("wake: %q", trimForLog(state.Phrase))
	}
	s.mu.Lock()
	if s.busy {
		s.mu.Unlock()
		s.logf("busy; new wake ignored")
		return
	}
	s.busy = true
	p := &pendingTurn{requestID: state.RequestID, wakeOnly: question == "", won: make(chan struct{})}
	s.pending = p
	s.mu.Unlock()
	go func() {
		defer func() {
			s.mu.Lock()
			s.busy = false
			if s.pending == p {
				s.pending = nil
			}
			s.mu.Unlock()
		}()
		s.turn(question, p)
	}()
}

func (s *service) turn(question string, p *pendingTurn) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Wake word only: there is no question, so there is nothing to wait for in
	// the cloud. Cancel Alice right away - on the bare wake word she likes to
	// start music - then pick exactly one station to say the acknowledgement.
	if question == "" {
		if s.pulse != nil {
			s.pulse.pulseFor(time.Duration(s.brain.cfg.FollowUpSeconds)*time.Second+2*time.Second, 140)
		}
		if !s.dry {
			if err := s.cli.cancel(); err != nil {
				s.logf("cancel: %v", err)
			}
		}
		// The station the cloud replies to is the one the stock multi-room
		// logic considers nearest, so prefer that verdict and use the local
		// election only when the cloud stays silent.
		ackWait := time.Duration(s.brain.cfg.CloudWaitMS) * time.Millisecond
		if ackWait > 600*time.Millisecond {
			ackWait = 600 * time.Millisecond
		}
		ack := false
		select {
		case <-p.won:
			ack = true
		case <-time.After(ackWait):
		case <-ctx.Done():
			return
		}
		if !ack && !s.arb.wins(250*time.Millisecond) {
			s.logf("wake ack: another station is elected")
			return
		}
		if s.dry {
			return
		}
		// Every station starts listening, so the question itself can be
		// answered by whichever station the cloud picks as nearest.
		if err := s.tts.speak(ctx, "Слушаю."); err != nil {
			s.logf("speak: %v", err)
		}
		if err := s.cli.startConversation(); err != nil {
			s.logf("start conversation: %v", err)
			return
		}
		s.armFollowUp(time.Duration(s.brain.cfg.FollowUpSeconds) * time.Second)
		s.logf("listening for the question for %ds", s.brain.cfg.FollowUpSeconds)
		return
	}

	// All stations hear the same phrase, but the cloud replies to exactly one
	// of them: the stock multi-room "nearest speaker" decision. Wait for that
	// verdict and stay silent when another station was chosen, so two speakers
	// never answer at once. Alice is cancelled the moment the reply arrives.
	wait := time.Duration(s.brain.cfg.CloudWaitMS) * time.Millisecond
	won := false
	select {
	case <-p.won:
		won = true
	case <-time.After(wait):
		s.logf("arbitration: no cloud reply in %s; staying silent", wait)
	case <-ctx.Done():
		return
	}
	if !won {
		return
	}

	if s.pulse != nil {
		s.pulse.pulseFor(900*time.Millisecond, 140)
	}
	stopThinking := func() {}
	if s.pulse != nil {
		stopThinking = s.pulse.think(ctx)
	}
	defer stopThinking()

	if err := s.budget.take(time.Now()); err != nil {
		s.logf("budget: %v", err)
		if !s.dry {
			s.tts.speak(ctx, "Дневной лимит обращений исчерпан. Попробуйте завтра.")
		}
		return
	}
	start := time.Now()
	answer, err := s.brain.ask(ctx, question)
	if err != nil {
		s.logf("ask failed after %s: %v", time.Since(start).Round(time.Millisecond), err)
		if !s.dry {
			s.tts.speak(ctx, "Сервис ответа сейчас недоступен. Попробуйте чуть позже.")
		}
		return
	}
	if answer == "" {
		s.logf("model returned no spoken answer")
		return
	}
	s.logf("answer in %s: %q", time.Since(start).Round(time.Millisecond), trimForLog(answer))
	if s.dry {
		return
	}
	if err := s.tts.speak(ctx, answer); err != nil {
		s.logf("speak: %v", err)
	}
}

func speechEstimate(text string) time.Duration {
	n := float64(len([]rune(text)))
	d := time.Duration((n/13.0 + 0.9) * float64(time.Second))
	if d < 1500*time.Millisecond {
		d = 1500 * time.Millisecond
	}
	if d > 25*time.Second {
		d = 25 * time.Second
	}
	return d
}

func trimForLog(s string) string {
	if os.Getenv("STATION_AGENT_LOG_CONTENT") != "1" {
		return "[redacted]"
	}
	if len([]rune(s)) <= 160 {
		return s
	}
	return string([]rune(s)[:160]) + "…"
}

// dumpStream records every IPC frame with a timestamp and the message kind.
// It is a diagnostics helper for discovering fields the bridge does not use.
func dumpStream(path string, seconds int, logf func(string, ...any)) error {
	conn, err := net.DialTimeout("tcp", ipcAddress, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Duration(seconds) * time.Second))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	logf("dumping frames for %ds to %s", seconds, path)
	for {
		payload, err := readFrame(conn)
		if err != nil {
			return nil
		}
		kind := "?"
		if root, perr := parseMessage(payload); perr == nil {
			if k, ok := root.int(253); ok {
				kind = fmt.Sprintf("%d", k)
			}
		}
		hexs := hex.EncodeToString(payload)
		if len(hexs) > 8192 {
			hexs = hexs[:8192] + "..."
		}
		fmt.Fprintf(f, "%s kind=%s len=%d hex=%s\n", time.Now().Format("15:04:05.000"), kind, len(payload), hexs)
	}
}

func observe(seconds int, logf func(string, ...any)) error {
	conn, err := net.DialTimeout("tcp", ipcAddress, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Duration(seconds) * time.Second))
	logf("read-only listen for %ds", seconds)
	for {
		payload, err := readFrame(conn)
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(err.Error(), "timeout") {
				return nil
			}
			return err
		}
		state, ok := decodeAliceState(payload)
		if !ok {
			continue
		}
		question, addressed := splitWake(state.Phrase, defaultWakeWords)
		logf("final=%v phrase=%q jarvis=%v question=%q", state.Final, trimForLog(state.Phrase), addressed, trimForLog(question))
	}
}

func openLog(path string) (func(string, ...any), func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, nil, err
	}
	if info, err := os.Stat(path); err == nil && info.Size() > 1<<20 {
		os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, nil, err
	}
	var mu sync.Mutex
	logf := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		mu.Lock()
		fmt.Fprintf(f, "%s %s\n", time.Now().Format("2006-01-02 15:04:05"), line)
		mu.Unlock()
	}
	return logf, func() { f.Close() }, nil
}
