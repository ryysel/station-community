package main

// Voice output. The station's own text-query channel always prefixes the
// answer with a service phrase ("Окей, повторяю за вами: ..."), so the
// default path synthesises the answer separately and plays it through the
// shared ALSA dmix device. The native voice stays as a fallback.

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hajimehoshi/go-mp3"
)

const gttsUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

type speaker struct {
	provider string
	cli      *nativeCLI
	http     *http.Client
	logf     func(string, ...any)
}

func newSpeaker(provider string, cli *nativeCLI, logf func(string, ...any)) *speaker {
	if provider == "" {
		provider = "gtts"
	}
	return &speaker{
		provider: provider,
		cli:      cli,
		http:     &http.Client{Timeout: 15 * time.Second},
		logf:     logf,
	}
}

// speak blocks until the answer has been spoken (or the context ends).
func (s *speaker) speak(ctx context.Context, text string) error {
	text = speechSafe(text)
	if text == "" {
		return errors.New("empty speech")
	}
	if s.provider == "native" {
		return s.speakNative(ctx, text)
	}
	if err := s.speakGoogle(ctx, text); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.logf("online voice unavailable (%v); falling back to native voice", err)
		return s.speakNative(ctx, text)
	}
	return nil
}

func (s *speaker) speakNative(ctx context.Context, text string) error {
	if err := s.cli.speak(text); err != nil {
		return err
	}
	select {
	case <-time.After(speechEstimate(text)):
	case <-ctx.Done():
	}
	return nil
}

func (s *speaker) speakGoogle(ctx context.Context, text string) error {
	chunks := splitForTTS(text, 180)
	type result struct {
		pcm  []byte
		rate int
		err  error
	}
	results := make([]result, len(chunks))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		go func(i int, chunk string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pcm, rate, err := s.gttsChunk(ctx, chunk)
			results[i] = result{pcm: pcm, rate: rate, err: err}
		}(i, chunk)
	}
	wg.Wait()

	rate := 0
	var pcm []byte
	for i, r := range results {
		if r.err != nil {
			return fmt.Errorf("chunk %d: %w", i+1, r.err)
		}
		if rate == 0 {
			rate = r.rate
		}
		pcm = append(pcm, r.pcm...)
	}
	if len(pcm) == 0 {
		return errors.New("empty audio")
	}
	if rate == 0 {
		rate = 24000
	}
	// go-mp3 always emits interleaved 16-bit stereo, even for mono sources.
	const channels = 2
	seconds := float64(len(pcm)) / float64(rate*channels*2)
	s.logf("voice: %d chunk(s), %.1fs, %d Hz", len(chunks), seconds, rate)
	return playPCM(ctx, pcm, rate, channels)
}

func (s *speaker) gttsChunk(ctx context.Context, text string) ([]byte, int, error) {
	q := url.Values{}
	q.Set("ie", "UTF-8")
	q.Set("client", "tw-ob")
	q.Set("tl", "ru")
	q.Set("ttsspeed", "1")
	q.Set("q", text)
	endpoint := "https://translate.google.com/translate_tts?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("User-Agent", gttsUserAgent)
	res, err := s.http.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return nil, 0, fmt.Errorf("provider HTTP %d", res.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, 0, err
	}
	dec, err := mp3.NewDecoder(bytes.NewReader(raw))
	if err != nil {
		return nil, 0, err
	}
	var pcm []byte
	buf := make([]byte, 32*1024)
	for {
		n, readErr := dec.Read(buf)
		if n > 0 {
			pcm = append(pcm, buf[:n]...)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, 0, readErr
		}
	}
	return pcm, dec.SampleRate(), nil
}

// playPCM writes a temporary WAV and plays it through the shared dmix
// device. No ALSA configuration or Quasar file is touched.
func playPCM(ctx context.Context, pcm []byte, rate, channels int) error {
	f, err := os.CreateTemp("/tmp", "jarvis-tts-*.wav")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err := writeWAV(f, pcm, rate, channels); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "aplay", "-q", "-D", "default", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("aplay: %v %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func writeWAV(w io.Writer, pcm []byte, rate, channels int) error {
	const bits = 16
	header := make([]byte, 44)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+len(pcm)))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(header[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(header[28:32], uint32(rate*channels*bits/8))
	binary.LittleEndian.PutUint16(header[32:34], uint16(channels*bits/8))
	binary.LittleEndian.PutUint16(header[34:36], bits)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(len(pcm)))
	if _, err := w.Write(header); err != nil {
		return err
	}
	_, err := w.Write(pcm)
	return err
}

// splitForTTS cuts text at sentence or word boundaries so every request
// stays inside the provider's limit.
func splitForTTS(text string, max int) []string {
	var out []string
	for len(text) > max {
		cut := -1
		for i := max; i > max/2; i-- {
			switch text[i] {
			case '.', '!', '?', ';', ':':
				cut = i + 1
			}
			if cut > 0 {
				break
			}
		}
		if cut < 0 {
			for i := max; i > max/2; i-- {
				if text[i] == ' ' || text[i] == ',' {
					cut = i + 1
					break
				}
			}
		}
		if cut <= 0 {
			cut = max
		}
		if piece := strings.TrimSpace(text[:cut]); piece != "" {
			out = append(out, piece)
		}
		text = strings.TrimSpace(text[cut:])
	}
	if text != "" {
		out = append(out, text)
	}
	return out
}
