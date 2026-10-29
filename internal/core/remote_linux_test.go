//go:build linux

package core

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestMain(m *testing.M) {
	if os.Getenv("SHELL_WORKFLOW_BINARY_CHILD") == "1" {
		b, e := os.ReadFile("/proc/self/exe")
		if e != nil {
			panic(e)
		}
		fmt.Println(string(b[len(b)-8:]))
		for {
			time.Sleep(time.Second)
		}
	}
	os.Exit(m.Run())
}
func localSSH(t *testing.T) Server {
	t.Helper()
	_, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	signer, e := ssh.NewSignerFromKey(private)
	if e != nil {
		t.Fatal(e)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "test" {
			return nil, fmt.Errorf("bad password")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		for {
			conn, e := listener.Accept()
			if e != nil {
				return
			}
			go func() {
				server, channels, requests, e := ssh.NewServerConn(conn, cfg)
				if e != nil {
					conn.Close()
					return
				}
				defer server.Close()
				go func() {
					for r := range requests {
						r.Reply(true, nil)
					}
				}()
				for ch := range channels {
					if ch.ChannelType() != "session" {
						ch.Reject(ssh.UnknownChannelType, "session only")
						continue
					}
					channel, reqs, e := ch.Accept()
					if e != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for req := range reqs {
							var payload struct{ Value string }
							ssh.Unmarshal(req.Payload, &payload)
							switch req.Type {
							case "pty-req", "window-change":
								req.Reply(true, nil)
							case "shell":
								req.Reply(true, nil)
								cmd := exec.Command("sh")
								stdin, _ := cmd.StdinPipe()
								go func() { io.Copy(stdin, channel); stdin.Close() }()
								cmd.Stdout = channel
								cmd.Stderr = channel.Stderr()
								_ = cmd.Run()
								channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
								return
							case "subsystem":
								req.Reply(true, nil)
								s, e := sftp.NewServer(channel)
								if e == nil {
									_ = s.Serve()
									s.Close()
								}
								return
							case "exec":
								req.Reply(true, nil)
								cmd := exec.Command("sh", "-c", payload.Value)
								cmd.Stdin = channel
								cmd.Stdout = channel
								cmd.Stderr = channel.Stderr()
								err := cmd.Run()
								code := uint32(0)
								if err != nil {
									code = 1
								}
								channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{code}))
								return
							default:
								req.Reply(false, nil)
							}
						}
					}()
				}
			}()
		}
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	p, _ := strconv.Atoi(port)
	return Server{Host: host, Port: p, User: "test", Auth: "password", Fingerprint: ssh.FingerprintSHA256(signer.PublicKey())}
}
func TestLinuxWorkingDirectoryAndNodeIsolation(t *testing.T) {
	s := localSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := Dial(ctx, s, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var baseline strings.Builder
	if err = r.Run(ctx, Step{Command: "pwd"}, func(v string) { baseline.WriteString(v) }); err != nil {
		t.Fatal(err)
	}
	a, b := t.TempDir(), t.TempDir()
	var out strings.Builder
	if err = r.Run(ctx, Step{Directory: a, Command: "pwd; cd " + Quote(b) + "; pwd"}, func(v string) { out.WriteString(v) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), a+"\n") || !strings.Contains(out.String(), b+"\n") {
		t.Fatal(out.String())
	}
	out.Reset()
	if err = r.Run(ctx, Step{Command: "pwd"}, func(v string) { out.WriteString(v) }); err != nil || out.String() != baseline.String() {
		t.Fatal("directory leaked across nodes", out.String(), err)
	}
	out.Reset()
	if err = r.Run(ctx, Step{Directory: filepath.Join(a, "missing"), Command: "true; printf 'should-not-run'"}, func(v string) { out.WriteString(v) }); err == nil || strings.Contains(out.String(), "should-not-run") {
		t.Fatal("script ran after failed cd", out.String(), err)
	}
}

func TestLinuxCommandModes(t *testing.T) {
	s := localSSH(t)
	e, err := NewEngine(t.TempDir(), func(ctx context.Context, s Server) (Session, error) { return Dial(ctx, s, "test", true) })
	if err != nil {
		t.Fatal(err)
	}
	a, b := t.TempDir(), t.TempDir()
	st := Step{Type: "command", CommandMode: "lines", Directory: a, Timeout: 5, Commands: []string{
		"SHELL_WORKFLOW_MODE_TEST=value; cd " + Quote(b),
		"printf 'value=%s\\n' \"${SHELL_WORKFLOW_MODE_TEST:-unset}\"; pwd",
		"false",
		"printf should-not-run",
	}}
	var output strings.Builder
	report := func(_, msg, _ string, _, _ int64) { output.WriteString(msg) }
	err = e.step(context.Background(), s, st, report)
	if err == nil || !strings.Contains(err.Error(), "第 3 条") || !strings.Contains(output.String(), "value=unset\n"+a+"\n") || strings.Contains(output.String(), "should-not-run") {
		t.Fatal(output.String(), err)
	}
	st.CommandMode = "script"
	st.Command = "set -e\nSHELL_WORKFLOW_MODE_TEST=value\ncd " + Quote(b) + "\nprintf 'value=%s\\n' \"$SHELL_WORKFLOW_MODE_TEST\"\npwd\nfalse\nprintf should-not-run"
	output.Reset()
	err = e.step(context.Background(), s, st, report)
	if err == nil || !strings.Contains(output.String(), "value=value\n"+b+"\n") || strings.Contains(output.String(), "should-not-run") {
		t.Fatal(output.String(), err)
	}
}

func TestLinuxExternalAssetDeploymentBranches(t *testing.T) {
	c, w := branchWorkflow()
	a, b := localSSH(t), localSSH(t)
	a.ID = "a"
	b.ID = "b"
	c.Servers = []Server{a, b}
	dirA, dirB, src := t.TempDir(), t.TempDir(), filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(src, []byte("shared-payload"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Artifacts[0].Files[0].Local = src
	w.Steps[1].Files[0].Remote = filepath.Join(dirA, "app")
	w.Steps[4].Files[0].Remote = filepath.Join(dirB, "app")
	w.Steps[2].Directory = dirA
	w.Steps[2].Command = "test -x app && cat app > restarted-by-supervisor"
	w.Steps[5].Directory = dirB
	w.Steps[5].Command = "sh -c 'test -x app && cat app > started-by-shell'"
	c.Workflows = []Workflow{w}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e, err := NewEngine(t.TempDir(), func(ctx context.Context, s Server) (Session, error) { return Dial(ctx, s, "test", true) })
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan Event, 1)
	e.OnEvent = func(ev Event) {
		if ev.Workflow == "" {
			finished <- ev
		}
	}
	if _, err = e.Start(ctx, c, Node{WorkflowID: w.ID}, "branches"); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-finished:
		if ev.Status != "success" {
			t.Fatal(ev)
		}
	case <-ctx.Done():
		t.Fatal("deployment timed out")
	}
	for _, file := range []string{filepath.Join(dirA, "restarted-by-supervisor"), filepath.Join(dirB, "started-by-shell")} {
		data, err := os.ReadFile(file)
		if err != nil || string(data) != "shared-payload" {
			t.Fatal(file, string(data), err)
		}
	}
}

func TestLinuxSameServerMultipleDeploymentDirectories(t *testing.T) {
	for _, batch := range []bool{true, false} {
		t.Run(map[bool]string{true: "one-upload", false: "parallel-branches"}[batch], func(t *testing.T) {
			c, w := branchWorkflow()
			s := localSSH(t)
			s.ID = "a"
			c.Servers = []Server{s}
			a, b := t.TempDir(), t.TempDir()
			src := filepath.Join(t.TempDir(), "app")
			if err := os.WriteFile(src, []byte("same-version"), 0600); err != nil {
				t.Fatal(err)
			}
			c.Artifacts[0].Files[0].Local = src
			w.Steps[1].Files[0].Remote = filepath.Join(a, "app")
			w.Steps[4].Files[0].Remote = filepath.Join(b, "app")
			w.Steps[2].Directory, w.Steps[2].Command = a, "test -x app && cat app > restarted"
			w.Steps[5].Directory, w.Steps[5].Command = b, "test -x app && cat app > restarted"
			if batch {
				w.Steps[1].Files = append(w.Steps[1].Files, w.Steps[4].Files[0])
				w.Steps = []Step{w.Steps[0], w.Steps[1], w.Steps[2], w.Steps[5]}
				w.Flow = nil
			} else {
				w.Steps = append(w.Steps[:3], w.Steps[4:]...)
				w.Flow = &Flow{Nodes: []FlowNode{{ID: "a", Ref: "a"}, {ID: "ua", Ref: "upload-a"}, {ID: "ra", Ref: "restart-a"}, {ID: "ub", Ref: "upload-b"}, {ID: "rb", Ref: "restart-b"}}, Edges: []FlowEdge{{"a", "ua"}, {"ua", "ra"}, {"a", "ub"}, {"ub", "rb"}}}
			}
			c.Workflows = []Workflow{w}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			e, err := NewEngine(t.TempDir(), func(ctx context.Context, server Server) (Session, error) { return Dial(ctx, server, "test", true) })
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			done := make(chan Event, 1)
			e.OnEvent = func(ev Event) {
				if ev.Workflow == "" {
					done <- ev
				}
			}
			if _, err := e.Start(ctx, c, Node{WorkflowID: w.ID}, "two-instances"); err != nil {
				t.Fatal(err)
			}
			select {
			case ev := <-done:
				if ev.Status != "success" {
					t.Fatal(ev)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			for _, dir := range []string{a, b} {
				data, err := os.ReadFile(filepath.Join(dir, "restarted"))
				if err != nil || string(data) != "same-version" {
					t.Fatal(dir, string(data), err)
				}
			}
		})
	}
}

func TestLinuxInteractiveTerminalAndDownload(t *testing.T) {
	s := localSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := Dial(ctx, s, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	terminal, err := OpenTerminal(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()
	if err = terminal.Resize(120, 35); err != nil {
		t.Fatal(err)
	}
	if err = terminal.Write("printf 'terminal-roundtrip-中文\\n'; exit\n"); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	for {
		part := terminal.Poll()
		b, _ := base64.StdEncoding.DecodeString(part.Data)
		output.Write(b)
		if part.Closed {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("terminal did not exit")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !strings.Contains(output.String(), "terminal-roundtrip-中文") {
		t.Fatal(output.String())
	}
	r, err = Dial(ctx, s, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "remote.txt")
	dst := filepath.Join(dir, "download.txt")
	os.WriteFile(src, []byte("downloaded bytes"), 0600)
	in, err := r.SFTP.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	if err = WriteAtomic(dst, func(out *os.File) error { _, err := io.Copy(out, in); return err }); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dst)
	if string(b) != "downloaded bytes" {
		t.Fatal(string(b))
	}
}

func TestLinuxRunningBinaryAtomicReplacement(t *testing.T) {
	for _, protocol := range []string{"sftp", "scp"} {
		t.Run(protocol, func(t *testing.T) { testLinuxRunningBinaryAtomicReplacement(t, protocol) })
	}
}

func testLinuxRunningBinaryAtomicReplacement(t *testing.T, protocol string) {
	s := localSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	r, e := Dial(ctx, s, "test", false)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	dir := t.TempDir()
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(self)
	if e != nil {
		t.Fatal(e)
	}
	target := filepath.Join(dir, "running-service")
	source := filepath.Join(dir, "new-service")
	if e = os.WriteFile(target, append(append([]byte{}, b...), []byte("VERSION1")...), 0755); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(source, append(b, []byte("VERSION2")...), 0755); e != nil {
		t.Fatal(e)
	}
	start := func() (*exec.Cmd, *bufio.Reader) {
		cmd := exec.Command(target)
		cmd.Env = append(os.Environ(), "SHELL_WORKFLOW_BINARY_CHILD=1")
		out, e := cmd.StdoutPipe()
		if e != nil {
			t.Fatal(e)
		}
		if e = cmd.Start(); e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
		return cmd, bufio.NewReader(out)
	}
	old, reader := start()
	line, e := reader.ReadString('\n')
	if e != nil || strings.TrimSpace(line) != "VERSION1" {
		t.Fatal(line, e)
	}
	if f, e := os.OpenFile(target, os.O_WRONLY|os.O_TRUNC, 0); e == nil {
		f.Close()
		t.Fatal("expected ETXTBSY for direct truncate")
	}
	files, e := Expand([]FileMapping{{Local: source, Remote: target}})
	if e != nil {
		t.Fatal(e)
	}
	published := 0
	e = r.Publish(ctx, files, Step{UploadProtocol: protocol}, func(_ string, _, _ int64, state string) {
		if state == "published" {
			published++
		}
	})
	if e != nil {
		t.Fatal(e)
	}
	if published != 1 {
		t.Fatal("not published")
	}
	if e = old.Process.Signal(syscall.Signal(0)); e != nil {
		t.Fatal("old process died", e)
	}
	_, reader = start()
	line, e = reader.ReadString('\n')
	if e != nil || strings.TrimSpace(line) != "VERSION2" {
		t.Fatal(line, e)
	}
	newHash, _ := FileHash(source)
	targetHash, _ := FileHash(target)
	if newHash != targetHash {
		t.Fatal("published hash mismatch")
	}
}

func TestLinuxDownloadBothProtocolsAndArchiveActions(t *testing.T) {
	s := localSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	source := filepath.Join(dir, "server ' files")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("server content\n"), 10000)
	file := filepath.Join(source, "data ' file.txt")
	if err := os.WriteFile(file, payload, 0755); err != nil {
		t.Fatal(err)
	}
	for _, protocol := range []string{"sftp", "scp"} {
		r, err := Dial(ctx, s, "test", false)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = r.Download(ctx, file, &out, protocol)
		r.Close()
		if err != nil || !bytes.Equal(out.Bytes(), payload) {
			t.Fatal("download mismatch", protocol, err)
		}
	}
	r, err := Dial(ctx, s, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	archive := filepath.Join(dir, "bundle.tar.gz")
	if err := r.ManageFile(ctx, RemoteFileOperation{Action: "compress", Source: source, Target: archive}, ""); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(dir, "extracted")
	if err := r.ManageFile(ctx, RemoteFileOperation{Action: "extract", Source: archive, Target: extracted}, ""); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(extracted, filepath.Base(source), filepath.Base(file)))
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("archive round trip failed", err)
	}
	if err := r.ManageFile(ctx, RemoteFileOperation{Action: "extract", Source: archive, Target: extracted}, ""); err == nil {
		t.Fatal("extraction overwrote an existing directory")
	}
	for _, unsafe := range []string{"../escape", "/tmp/escape", "folder/../../escape", "folder\\escape"} {
		var content bytes.Buffer
		writer := tar.NewWriter(&content)
		if err := writer.WriteHeader(&tar.Header{Name: unsafe, Mode: 0600, Size: 1}); err != nil {
			t.Fatal(err)
		}
		writer.Write([]byte("x"))
		writer.Close()
		bad := filepath.Join(dir, "unsafe.tar")
		os.WriteFile(bad, content.Bytes(), 0600)
		if err := r.ManageFile(ctx, RemoteFileOperation{Action: "extract", Source: bad, Target: filepath.Join(dir, "unsafe-out")}, ""); err == nil {
			t.Fatal("unsafe tar entry accepted", unsafe)
		}
	}
	var content bytes.Buffer
	zipWriter := zip.NewWriter(&content)
	entry, err := zipWriter.Create("nested/config.txt")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("zip content"))
	zipWriter.Close()
	zipped := filepath.Join(dir, "config.zip")
	os.WriteFile(zipped, content.Bytes(), 0600)
	if err := r.ManageFile(ctx, RemoteFileOperation{Action: "extract", Source: zipped, Target: filepath.Join(dir, "zip-out")}, ""); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "zip-out/nested/config.txt")); err != nil || string(got) != "zip content" {
		t.Fatal("ZIP extraction failed", err)
	}
}
func TestLinuxFailedPreparationKeepsOriginal(t *testing.T) {
	s := localSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := Dial(ctx, s, "test", false)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	dir := t.TempDir()
	src := filepath.Join(dir, "source")
	target := filepath.Join(dir, "target")
	os.WriteFile(src, []byte("new"), 0600)
	os.WriteFile(target, []byte("old"), 0600)
	files, _ := Expand([]FileMapping{{Local: src, Remote: target}})
	files = append(files, Artifact{Local: filepath.Join(dir, "missing"), Remote: filepath.Join(dir, "other")})
	if e = r.Publish(ctx, files, Step{}, func(string, int64, int64, string) {}); e == nil {
		t.Fatal("expected failure")
	}
	b, _ := os.ReadFile(target)
	if string(b) != "old" {
		t.Fatal("old target modified")
	}
	parts, _ := filepath.Glob(filepath.Join(dir, ".*.part"))
	if len(parts) != 0 {
		t.Fatal("temporary files leaked", parts)
	}
}
func TestLinuxCommandFailure(t *testing.T) {
	s := localSSH(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, e := Dial(ctx, s, "test", false)
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	if e = r.Run(ctx, Step{Command: "printf hello; exit 7"}, func(string) {}); e == nil {
		t.Fatal("nonzero exit accepted")
	}
	var out strings.Builder
	if e = r.Run(ctx, Step{Command: "printf '%s' " + Quote("a'b")}, func(s string) { io.WriteString(&out, s) }); e != nil || out.String() != "a'b" {
		t.Fatal(out.String(), e)
	}
}
