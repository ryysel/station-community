package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const (
	nativeBinary = "/system/vendor/quasar/maind"
	nativeLibs   = "/system/vendor/quasar/libs"
)

// nativeCLI keeps one resident `maind --cli` process. It is the documented
// local control channel of the station: text queries and cancellation.
// No ALSA configuration, Quasar files or loader globals are modified.
type nativeCLI struct {
	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	started bool
}

func (n *nativeCLI) startLocked() error {
	if n.cmd != nil && n.cmd.Process != nil && n.started {
		return nil
	}
	cmd := exec.Command(nativeBinary, "--cli")
	env := make([]string, 0, len(os.Environ())+1)
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "LD_LIBRARY_PATH=") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env, "LD_LIBRARY_PATH="+nativeLibs)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = cmd.Stdout
	if err = cmd.Start(); err != nil {
		return err
	}
	ready := make(chan struct{})
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 256*1024)
		signalled := false
		for scanner.Scan() {
			line := scanner.Text()
			if !signalled && strings.Contains(line, "Commands:") {
				signalled = true
				close(ready)
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		return errors.New("native CLI did not become ready")
	}
	n.cmd, n.stdin, n.started = cmd, stdin, true
	return nil
}

func (n *nativeCLI) ensure() error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.started && n.cmd != nil && n.cmd.ProcessState == nil {
		return nil
	}
	n.started = false
	return n.startLocked()
}

func (n *nativeCLI) write(line string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if !n.started || n.stdin == nil {
		return errors.New("native CLI is not running")
	}
	if _, err := io.WriteString(n.stdin, line); err != nil {
		n.started = false
		return err
	}
	return nil
}

func (n *nativeCLI) cancel() error {
	if err := n.ensure(); err != nil {
		return err
	}
	return n.write("c\n")
}

// startConversation puts the station back into listening mode, so the user
// can dictate the question right after the wake word.
func (n *nativeCLI) startConversation() error {
	if err := n.ensure(); err != nil {
		return err
	}
	return n.write("s\n")
}

func (n *nativeCLI) speak(text string) error {
	text = speechSafe(text)
	if text == "" {
		return errors.New("empty speech")
	}
	if err := n.ensure(); err != nil {
		return err
	}
	if err := n.write("q\n"); err != nil {
		return err
	}
	time.Sleep(60 * time.Millisecond)
	return n.write("повтори за мной " + text + "\n")
}

func (n *nativeCLI) close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stdin != nil {
		io.WriteString(n.stdin, "x\n")
		n.stdin.Close()
	}
	if n.cmd != nil && n.cmd.Process != nil {
		done := make(chan struct{})
		go func() { n.cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			n.cmd.Process.Kill()
		}
	}
	n.cmd, n.stdin, n.started = nil, nil, false
}

// speechSafe removes markup and keeps one line, so model text can never
// become an extra CLI command.
func speechSafe(value string) string {
	value = strings.NewReplacer("*", "", "_", "", "#", "", "`", "", "\r", " ", "\n", " ", "\t", " ").Replace(value)
	value = strings.Join(strings.Fields(value), " ")
	value = strings.TrimSpace(value)
	if len(value) > 700 {
		cut := value[:700]
		if i := strings.LastIndexAny(cut, ".!?"); i > 300 {
			cut = cut[:i+1]
		} else if i := strings.LastIndex(cut, " "); i > 0 {
			cut = cut[:i] + "."
		}
		value = cut
	}
	return value
}

func (n *nativeCLI) status() string {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.cmd != nil && n.cmd.Process != nil && n.cmd.ProcessState == nil {
		return fmt.Sprintf("running (pid %d)", n.cmd.Process.Pid)
	}
	return "not running"
}
