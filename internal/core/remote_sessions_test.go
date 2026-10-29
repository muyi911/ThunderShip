package core

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// A real SSH/SFTP peer with an in-memory filesystem, usable on Windows as well as Linux.
type reusePeer struct {
	server      Server
	files       sftp.Handlers
	connections atomic.Int32
	closed      atomic.Int32
	commands    atomic.Int32
	ptys        atomic.Int32
	transfers   atomic.Int32
	shortages   chan struct{}
	limit       int32
	limitReason ssh.RejectionReason
	denySudo    bool
	exitCode    uint32
	exec        func(context.Context, ssh.Channel, string)
	sftp        func(context.Context, ssh.Channel, int32) bool
	channel     func(context.Context, ssh.NewChannel) bool
	requests    func(context.Context, *ssh.ServerConn, <-chan *ssh.Request)
}

func newReusePeer(t testing.TB, peer *reusePeer) *reusePeer {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(private)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if string(password) != "test" {
			return nil, fmt.Errorf("invalid password")
		}
		return nil, nil
	}}
	cfg.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	portNumber, _ := strconv.Atoi(port)
	peer.server = Server{ID: "s", Host: host, Port: portNumber, User: "test", Auth: "password", Fingerprint: ssh.FingerprintSHA256(signer.PublicKey()), SFTPPath: "/usr/lib/sftp-server"}
	peer.files = sftp.InMemHandler()
	peer.shortages = make(chan struct{}, 100)
	if peer.limitReason == 0 {
		peer.limitReason = ssh.ResourceShortage
	}
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	var mu sync.Mutex
	conns := map[net.Conn]bool{}
	t.Cleanup(func() {
		cancel()
		listener.Close()
		mu.Lock()
		for conn := range conns {
			conn.Close()
		}
		mu.Unlock()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			if ctx.Err() != nil {
				mu.Unlock()
				conn.Close()
				return
			}
			conns[conn] = true
			mu.Unlock()
			peer.connections.Add(1)
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					conn.Close()
					mu.Lock()
					delete(conns, conn)
					mu.Unlock()
					peer.closed.Add(1)
				}()
				server, channels, requests, err := ssh.NewServerConn(conn, cfg)
				if err != nil {
					return
				}
				defer server.Close()
				if peer.requests != nil {
					go peer.requests(ctx, server, requests)
				} else {
					go ssh.DiscardRequests(requests)
				}
				var active atomic.Int32
				for ch := range channels {
					if peer.channel != nil && peer.channel(ctx, ch) {
						continue
					}
					if peer.limit > 0 && active.Load() >= peer.limit {
						ch.Reject(peer.limitReason, "session limit")
						peer.shortages <- struct{}{}
						continue
					}
					channel, requests, err := ch.Accept()
					if err != nil {
						continue
					}
					active.Add(1)
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer active.Add(-1)
						defer channel.Close()
						for request := range requests {
							var payload struct{ Value string }
							ssh.Unmarshal(request.Payload, &payload)
							isSudo := request.Type == "exec" && strings.HasPrefix(payload.Value, "sudo -n -u ") && !strings.Contains(payload.Value, " -- scp -t -- ")
							if request.Type == "subsystem" || isSudo {
								if isSudo && peer.denySudo {
									request.Reply(false, nil)
									return
								}
								request.Reply(true, nil)
								n := peer.transfers.Add(1)
								if peer.sftp != nil && peer.sftp(ctx, channel, n) {
									return
								}
								files := sftp.NewRequestServer(channel, peer.files)
								files.Serve()
								files.Close()
								return
							}
							if request.Type == "exec" {
								request.Reply(true, nil)
								peer.commands.Add(1)
								if peer.exec != nil {
									peer.exec(ctx, channel, payload.Value)
								}
								channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{peer.exitCode}))
								return
							}
							if request.Type == "pty-req" || request.Type == "window-change" {
								if request.Type == "pty-req" {
									peer.ptys.Add(1)
								}
								request.Reply(true, nil)
								continue
							}
							request.Reply(false, nil)
						}
					}()
				}
			}()
		}
	}()
	return peer
}

func TestSSHRunParallelUploadsJoinAndNestedWorkflowUseOneConnection(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	e, err := NewEngine(t.TempDir(), func(ctx context.Context, s Server) (Session, error) { return DialSSH(ctx, s, "test") })
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(file, []byte("binary"), 0600); err != nil {
		t.Fatal(err)
	}
	c := EmptyConfig()
	c.Servers = []Server{peer.server}
	w := NormalizeWorkflow(workflow("w", "restart"))
	w.Steps = []Step{w.Steps[0],
		{ID: "upload-a", Type: "upload", Timeout: 3, Files: []FileMapping{{ID: "a", Local: file, Remote: "/a/app"}}},
		{ID: "upload-b", Type: "upload", Sudo: true, Timeout: 3, Files: []FileMapping{{ID: "b", Local: file, Remote: "/b/app"}}},
		{ID: "restart", Type: "command", Command: "restart", Timeout: 3},
		{ID: "call", Type: "workflow", WorkflowID: "child"},
	}
	w.Flow = &Flow{Nodes: []FlowNode{{ID: "target", Ref: w.Steps[0].ID}, {ID: "a", Ref: "upload-a"}, {ID: "b", Ref: "upload-b"}, {ID: "restart", Ref: "restart"}, {ID: "call", Ref: "call"}}, Edges: []FlowEdge{{"target", "a"}, {"target", "b"}, {"a", "restart"}, {"b", "restart"}, {"restart", "call"}}}
	c.Workflows = []Workflow{w, workflow("child", "verify")}
	var filesChecked atomic.Int32
	peer.exec = func(_ context.Context, _ ssh.Channel, command string) {
		if !strings.Contains(command, "restart") {
			return
		}
		for _, path := range []string{"/a/app", "/b/app"} {
			reader, err := peer.files.FileGet.Fileread(&sftp.Request{Method: "Get", Filepath: path, Flags: 1})
			if err != nil {
				t.Error("restart ran before both uploads", path, err)
				continue
			}
			buf := make([]byte, 6)
			_, err = reader.ReadAt(buf, 0)
			if err != nil && err != io.EOF || string(buf) != "binary" {
				t.Error("wrong published payload", string(buf), err)
			}
			filesChecked.Add(1)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := e.node(ctx, c, Node{WorkflowID: "w"}, "run"); err != nil {
		t.Fatal(err)
	}
	if peer.connections.Load() != 1 || peer.transfers.Load() != 2 || peer.commands.Load() != 2 || filesChecked.Load() != 2 {
		t.Fatal("unexpected connection/channel counts", peer.connections.Load(), peer.transfers.Load(), peer.commands.Load(), filesChecked.Load())
	}
}

func TestSSHTransferInitializationCancellationDoesNotCloseSibling(t *testing.T) {
	started := make(chan struct{})
	peer := newReusePeer(t, &reusePeer{sftp: func(_ context.Context, ch ssh.Channel, n int32) bool {
		if n != 1 {
			return false
		}
		close(started)
		io.Copy(io.Discard, ch)
		return true
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	short, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { _, err := r.OpenTransfer(short, peer.server); done <- err }()
	<-started
	sibling, err := r.OpenTransfer(ctx, peer.server)
	if err != nil {
		t.Fatal(err)
	}
	defer sibling.Close()
	stop()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled SFTP initialization succeeded")
		}
	case <-ctx.Done():
		t.Fatal("canceled SFTP initialization stalled")
	}
	if err := sibling.CheckFiles([]Artifact{{Remote: "/app"}}); err != nil {
		t.Fatal("canceled upload broke sibling SFTP", err)
	}
	if err := r.Run(ctx, Step{Command: "still-alive"}, nil); err != nil || peer.connections.Load() != 1 {
		t.Fatal("transport lost after upload cancellation", err)
	}
}

func TestSSHSudoFailureAndCommandTimeoutKeepTransportUsable(t *testing.T) {
	started := make(chan struct{})
	peer := newReusePeer(t, &reusePeer{denySudo: true, exec: func(ctx context.Context, ch ssh.Channel, cmd string) {
		if strings.Contains(cmd, "long-running") {
			close(started)
			io.Copy(io.Discard, ch)
		}
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	s := peer.server
	s.SudoUpload = true
	if _, err := r.OpenTransfer(ctx, s); err == nil {
		t.Fatal("sudo failure accepted")
	}
	sibling, err := r.OpenTransfer(ctx, peer.server)
	if err != nil {
		t.Fatal("sudo failure closed transport", err)
	}
	defer sibling.Close()
	short, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- r.Run(short, Step{Command: "long-running"}, nil) }()
	<-started
	stop()
	var unknown *UnknownError
	if err := <-done; !errors.As(err, &unknown) {
		t.Fatal("canceled command lost unknown status", err)
	}
	if err := r.Run(ctx, Step{Command: "independent"}, nil); err != nil {
		t.Fatal("canceled command closed shared transport", err)
	}
	if err := sibling.CheckFiles([]Artifact{{Remote: "/app"}}); err != nil || peer.connections.Load() != 1 || peer.commands.Load() != 2 {
		t.Fatal("cancellation broke sibling or replayed command", err)
	}
}

func TestSSHChannelLimitWaitsWithoutAnotherConnection(t *testing.T) {
	for _, reason := range []ssh.RejectionReason{ssh.ResourceShortage, ssh.ConnectionFailed} {
		t.Run(fmt.Sprint(reason), func(t *testing.T) {
			peer := newReusePeer(t, &reusePeer{limit: 1, limitReason: reason})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r, err := DialSSH(ctx, peer.server, "test")
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			transfer, err := r.OpenTransfer(ctx, peer.server)
			if err != nil {
				t.Fatal(err)
			}
			defer transfer.Close()
			done := make(chan error, 1)
			go func() { done <- r.Run(ctx, Step{Command: "after-channel-release"}, nil) }()
			select {
			case <-peer.shortages:
			case <-ctx.Done():
				t.Fatal("second node did not request a channel")
			}
			transfer.Close()
			select {
			case err := <-done:
				if err != nil || peer.connections.Load() != 1 || peer.commands.Load() != 1 {
					t.Fatal("channel limit opened another connection or replayed command", err)
				}
			case <-ctx.Done():
				t.Fatal("channel limit never recovered")
			}
		})
	}
}

func TestSSHStandaloneDialStillSupportsSFTPAndCommands(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, err := Dial(ctx, peer.server, "test", true)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.CheckFiles([]Artifact{{Remote: "/app"}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(ctx, Step{Command: "pwd"}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSSHScopeReplacesClosedTransportForLaterNodes(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	e, err := NewEngine(t.TempDir(), func(ctx context.Context, s Server) (Session, error) { return DialSSH(ctx, s, "test") })
	if err != nil {
		t.Fatal(err)
	}
	ctx, closeSessions := e.withSessions(ctx)
	defer closeSessions()
	pool := ctx.Value(sessionScopeKey{}).(*sessionScope)
	first, err := pool.get(ctx, peer.server)
	if err != nil {
		t.Fatal(err)
	}
	first.(*Remote).SSH.Close()
	select {
	case <-first.(*Remote).closed:
	case <-ctx.Done():
		t.Fatal("closed transport was not detected")
	}
	second, err := pool.get(ctx, peer.server)
	if err != nil || first == second {
		t.Fatal("later node received a closed transport", err)
	}
	if err := second.Run(ctx, Step{Command: "later-node"}, nil); err != nil || peer.connections.Load() != 2 || peer.commands.Load() != 1 {
		t.Fatal("replacement connection failed or replayed a command", err)
	}
}

func TestSSHPublishBeforeCommandWithOneAllowedChannel(t *testing.T) {
	for _, exitCode := range []uint32{0, 1} {
		t.Run(fmt.Sprint(exitCode), func(t *testing.T) {
			peer := newReusePeer(t, &reusePeer{limit: 1, limitReason: ssh.ConnectionFailed, exitCode: exitCode})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r, err := DialSSH(ctx, peer.server, "test")
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			transfer, err := r.OpenTransfer(ctx, peer.server)
			if err != nil {
				t.Fatal(err)
			}
			defer transfer.Close()
			local := filepath.Join(t.TempDir(), "app")
			if err := os.WriteFile(local, []byte("binary"), 0600); err != nil {
				t.Fatal(err)
			}
			var published bool
			err = transfer.Publish(ctx, []Artifact{{Local: local, Remote: "/app", Size: 6}}, Step{Command: "stop-app"}, func(_ string, _, _ int64, status string) {
				published = published || status == "published"
			})
			if (err == nil) != (exitCode == 0) || published != (exitCode == 0) || peer.connections.Load() != 1 || peer.commands.Load() != 1 || peer.transfers.Load() != 2 {
				t.Fatal("before-publish command deadlocked or opened another connection", err)
			}
			check, err := r.OpenTransfer(ctx, peer.server)
			if err != nil {
				t.Fatal(err)
			}
			defer check.Close()
			entries, err := check.(*Remote).SFTP.ReadDir("/")
			if err != nil || (exitCode == 0 && len(entries) != 1) || (exitCode != 0 && len(entries) != 0) {
				t.Fatal("temporary upload files left after before-publish command", entries, err)
			}
		})
	}
}
