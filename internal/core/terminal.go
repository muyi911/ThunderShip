package core

import (
	"context"
	"encoding/base64"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type Terminal struct {
	Remote  *Remote
	session *ssh.Session
	input   io.WriteCloser
	cancel  context.CancelFunc
	ctx     context.Context
	output  chan []byte
	done    chan struct{}
	mu      sync.Mutex
	inputMu sync.Mutex
	err     string
	once    sync.Once
}
type TerminalRead struct {
	Data   string `json:"data"`
	Closed bool   `json:"closed"`
	Error  string `json:"error"`
}
type terminalWriter struct{ t *Terminal }

func (w terminalWriter) Write(b []byte) (int, error) {
	data := append([]byte(nil), b...)
	select {
	case w.t.output <- data:
		return len(b), nil
	case <-w.t.ctx.Done():
		return 0, w.t.ctx.Err()
	}
}
func OpenTerminal(ctx context.Context, r *Remote) (*Terminal, error) {
	ctx, cancel := context.WithCancel(ctx)
	t := &Terminal{Remote: r, ctx: ctx, cancel: cancel, output: make(chan []byte, 128), done: make(chan struct{})}
	s, err := r.SSH.NewSession()
	if err != nil {
		cancel()
		r.Close()
		return nil, err
	}
	t.session = s
	t.input, err = s.StdinPipe()
	if err == nil {
		err = s.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400})
	}
	if err != nil {
		t.Close()
		return nil, err
	}
	s.Stdout = terminalWriter{t}
	s.Stderr = terminalWriter{t}
	if err = s.Shell(); err != nil {
		t.Close()
		return nil, err
	}
	go func() {
		err := s.Wait()
		t.mu.Lock()
		if err != nil {
			t.err = err.Error()
		}
		t.mu.Unlock()
		close(t.done)
		t.Close()
	}()
	return t, nil
}
func (t *Terminal) Write(data string) error {
	t.inputMu.Lock()
	defer t.inputMu.Unlock()
	_, err := io.WriteString(t.input, data)
	return err
}
func (t *Terminal) Resize(cols, rows int) error {
	if cols < 2 || cols > 1000 || rows < 2 || rows > 500 {
		return nil
	}
	return t.session.WindowChange(rows, cols)
}
func (t *Terminal) Poll() TerminalRead {
	return t.poll(nil)
}

// Wait for output instead of adding a polling interval to every key's echo.
func (t *Terminal) WaitPoll() TerminalRead {
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	var first []byte
	select {
	case first = <-t.output:
	case <-t.done:
	case <-t.ctx.Done():
	case <-timer.C:
	}
	return t.poll(first)
}
func (t *Terminal) poll(data []byte) TerminalRead {
	result := TerminalRead{}
	// Observe completion before draining so the final output cannot be dropped.
	select {
	case <-t.done:
		result.Closed = true
	default:
	}
	for i := 0; i < 128; i++ {
		select {
		case b := <-t.output:
			data = append(data, b...)
		default:
			i = 128
		}
	}
	t.mu.Lock()
	err := t.err
	t.mu.Unlock()
	result.Data = base64.StdEncoding.EncodeToString(data)
	result.Error = err
	return result
}
func (t *Terminal) Close() {
	t.once.Do(func() {
		t.cancel()
		if t.session != nil {
			t.session.Close()
		}
		t.Remote.Close()
	})
}
