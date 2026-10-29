package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// Each enhanced command gets its own channel on the existing SSH transport.
type TerminalCommand struct {
	session   *ssh.Session
	input     io.WriteCloser
	inputMu   sync.Mutex
	mu        sync.Mutex
	cancel    context.CancelFunc
	done      chan struct{}
	pending   []byte
	held      []byte
	marker    string
	directory string
	closed    bool
	err       string
	exitCode  int
	started   time.Time
	duration  int64
	truncated bool
}
type CommandRead struct {
	Data      string `json:"data"`
	Closed    bool   `json:"closed"`
	Error     string `json:"error"`
	ExitCode  int    `json:"exitCode"`
	Directory string `json:"directory"`
	Duration  int64  `json:"duration"`
}

func commandScript(command, directory, marker string) string {
	trap := "_thunder_status=$?; printf '\\036" + marker + ":%s\\037' \"$PWD\"; exit \"$_thunder_status\""
	return "sh -c " + Quote("cd -- "+Quote(directory)+" || exit $?\ntrap "+Quote(trap)+" EXIT\neval "+Quote(command))
}

func StartTerminalCommand(ctx context.Context, client *ssh.Client, command, directory string) (*TerminalCommand, error) {
	if strings.TrimSpace(command) == "" || len(command) > 128*1024 || strings.ContainsRune(command, 0) {
		return nil, fmt.Errorf("命令为空、过长或包含无效字符")
	}
	s, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	c := &TerminalCommand{cancel: cancel, done: make(chan struct{}), marker: ID(), directory: directory, started: time.Now(), exitCode: -1}
	c.session = s
	c.input, err = s.StdinPipe()
	if err == nil {
		err = s.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{ssh.ECHO: 0, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400})
	}
	if err != nil {
		cancel()
		s.Close()
		return nil, err
	}
	s.Stdout, s.Stderr = c, c
	if err = s.Start(commandScript(command, directory, c.marker)); err != nil {
		cancel()
		s.Close()
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			s.Signal(ssh.SIGTERM)
			s.Close()
		case <-c.done:
		}
	}()
	go func() {
		err := s.Wait()
		c.mu.Lock()
		c.closed = true
		c.duration = time.Since(c.started).Milliseconds()
		c.exitCode = 0
		if err != nil {
			var status *ssh.ExitError
			if errors.As(err, &status) {
				c.exitCode = status.ExitStatus()
			} else {
				c.exitCode = -1
				c.err = err.Error()
			}
		}
		if ctx.Err() != nil {
			c.err = "命令已取消或超时；远端进程状态请核对"
			c.exitCode = -1
		}
		c.append(c.held)
		c.held = nil
		c.mu.Unlock()
		close(c.done)
		cancel()
		s.Close()
	}()
	return c, nil
}

func (c *TerminalCommand) append(data []byte) {
	const limit = 1024 * 1024
	available := limit - len(c.pending)
	if len(data) > available {
		data = data[:available]
		c.truncated = true
	}
	c.pending = append(c.pending, data...)
}
func (c *TerminalCommand) Write(data []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	size := len(data)
	c.held = append(c.held, data...)
	prefix := []byte("\x1e" + c.marker + ":")
	for len(c.held) > 0 {
		pos := bytes.IndexByte(c.held, 30)
		if pos < 0 {
			c.append(c.held)
			c.held = nil
			break
		}
		c.append(c.held[:pos])
		c.held = c.held[pos:]
		if len(c.held) < len(prefix) && bytes.HasPrefix(prefix, c.held) {
			break
		}
		if bytes.HasPrefix(c.held, prefix) {
			end := bytes.IndexByte(c.held[len(prefix):], 31)
			if end < 0 && len(c.held) < 8192 {
				break
			}
			if end >= 0 {
				c.directory = string(c.held[len(prefix) : len(prefix)+end])
				c.held = c.held[len(prefix)+end+1:]
				continue
			}
		}
		c.append(c.held[:1])
		c.held = c.held[1:]
	}
	return size, nil
}
func (c *TerminalCommand) Poll() CommandRead {
	c.mu.Lock()
	defer c.mu.Unlock()
	data := c.pending
	c.pending = nil
	if c.truncated {
		data = append(data, []byte("\r\n[输出过快，部分内容已截断]\r\n")...)
		c.truncated = false
	}
	duration := c.duration
	if !c.closed {
		duration = time.Since(c.started).Milliseconds()
	}
	return CommandRead{base64.StdEncoding.EncodeToString(data), c.closed, c.err, c.exitCode, c.directory, duration}
}
func (c *TerminalCommand) Close() { c.cancel() }

func (c *TerminalCommand) Input(data string) error {
	c.inputMu.Lock()
	defer c.inputMu.Unlock()
	_, err := io.WriteString(c.input, data)
	return err
}

func (c *TerminalCommand) Resize(cols, rows int) error {
	if cols < 2 || cols > 1000 || rows < 2 || rows > 500 {
		return fmt.Errorf("终端尺寸无效")
	}
	return c.session.WindowChange(rows, cols)
}
