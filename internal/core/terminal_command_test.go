package core

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func commandShell(t *testing.T) string {
	t.Helper()
	if shell, err := exec.LookPath("sh"); err == nil {
		return shell
	}
	shell := "C:/Program Files/Git/bin/bash.exe"
	if _, err := os.Stat(shell); err != nil {
		t.Skip("shell unavailable")
	}
	return shell
}
func TestEnhancedCommandScriptAndChunkedMetadata(t *testing.T) {
	shell := commandShell(t)
	for _, test := range []struct {
		command string
		code    int
		output  string
	}{
		{"printf '中文\\n'; cd /tmp", 0, "中文\n"},
		{"printf '%s\\n' \"a'b\"; exit 7", 7, "a'b\n"},
		{"set -e; false; printf wrong", 1, ""},
	} {
		cmd := exec.Command(shell, "-c", commandScript(test.command, "/", "marker"))
		data, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			if status, ok := err.(*exec.ExitError); ok {
				code = status.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if code != test.code {
			t.Fatalf("exit %d want %d: %s", code, test.code, data)
		}
		for size := 1; size <= len(data); size++ {
			c := &TerminalCommand{marker: "marker", directory: "/", started: time.Now()}
			var out strings.Builder
			for offset := 0; offset < len(data); offset += size {
				end := min(offset+size, len(data))
				c.Write(data[offset:end])
				result := c.Poll()
				decoded, _ := base64.StdEncoding.DecodeString(result.Data)
				out.Write(decoded)
			}
			if out.String() != test.output || strings.Contains(c.directory, "marker") {
				t.Fatalf("chunk %d: %q dir %q", size, out.String(), c.directory)
			}
			if test.code == 0 && !strings.HasSuffix(c.directory, "/tmp") {
				t.Fatal("cd not retained", c.directory)
			}
		}
	}
}
func TestEnhancedParallelChannelsCancelDoesNotCloseTransport(t *testing.T) {
	shell := commandShell(t)
	peer := newReusePeer(t, &reusePeer{exec: func(ctx context.Context, ch ssh.Channel, command string) {
		if strings.Contains(command, "slow-command") {
			for {
				if _, err := io.WriteString(ch, "working\n"); err != nil {
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
			return
		}
		cmd := exec.CommandContext(ctx, shell, "-c", command)
		cmd.Stdout = ch
		cmd.Stderr = ch.Stderr()
		_ = cmd.Run()
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	remote, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	slow, err := StartTerminalCommand(ctx, remote.SSH, "echo slow-command", "/")
	if err != nil {
		t.Fatal(err)
	}
	fast, err := StartTerminalCommand(ctx, remote.SSH, "printf success; cd /tmp", "/")
	if err != nil {
		t.Fatal(err)
	}
	slow.Close()
	select {
	case <-slow.done:
	case <-ctx.Done():
		t.Fatal("cancel blocked")
	}
	select {
	case <-fast.done:
	case <-ctx.Done():
		t.Fatal("sibling blocked")
	}
	result := fast.Poll()
	data, _ := base64.StdEncoding.DecodeString(result.Data)
	if string(data) != "success" || result.ExitCode != 0 || !strings.HasSuffix(result.Directory, "/tmp") || peer.connections.Load() != 1 {
		t.Fatalf("unexpected result %+v %q connections %d", result, data, peer.connections.Load())
	}
	next, err := StartTerminalCommand(ctx, remote.SSH, "echo alive", "/")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-next.done:
	case <-ctx.Done():
		t.Fatal("transport did not survive")
	}
	if slow.Poll().Error == "" {
		t.Fatal("cancel must be reported")
	}
}
func TestEnhancedOutputBoundedAndMalformedMarkerPreserved(t *testing.T) {
	c := &TerminalCommand{marker: "marker", started: time.Now()}
	c.Write([]byte("text\x1eother:dir\x1f"))
	result := c.Poll()
	data, _ := base64.StdEncoding.DecodeString(result.Data)
	if string(data) != "text\x1eother:dir\x1f" {
		t.Fatal("ordinary output changed")
	}
	c.Write([]byte(strings.Repeat("a", 2*1024*1024)))
	if len(c.pending) > 1024*1024 || !c.truncated {
		t.Fatal("unbounded output")
	}
}

func TestEnhancedInteractiveCommandInput(t *testing.T) {
	input := "ihello\x1b:wq\r"
	peer := newReusePeer(t, &reusePeer{exec: func(ctx context.Context, ch ssh.Channel, command string) {
		io.WriteString(ch, "\x1b[?1049heditor ready")
		data := make([]byte, len(input))
		if _, err := io.ReadFull(ch, data); err == nil {
			ch.Write(data)
			io.WriteString(ch, "\x1b[?1049l")
		}
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	c, err := StartTerminalCommand(ctx, r.SSH, "vi test.conf", "/")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err = c.Resize(120, 40); err != nil {
		t.Fatal(err)
	}
	if err = c.Resize(0, 40); err == nil {
		t.Fatal("invalid size accepted")
	}
	if err = c.Input(input); err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.done:
	case <-ctx.Done():
		t.Fatal("interactive input did not reach running command")
	}
	result := c.Poll()
	data, _ := base64.StdEncoding.DecodeString(result.Data)
	if !result.Closed || result.ExitCode != 0 || string(data) != "\x1b[?1049heditor ready"+input+"\x1b[?1049l" || peer.commands.Load() != 1 || peer.ptys.Load() != 1 {
		t.Fatalf("interactive command was changed or run twice: %+v %q", result, data)
	}
}
