package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func tunnelPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func tunnelTestPeer(t *testing.T) *reusePeer {
	bridge := func(ctx context.Context, local net.Conn, channel ssh.Channel) {
		defer local.Close()
		defer channel.Close()
		stop := context.AfterFunc(ctx, func() { local.Close(); channel.Close() })
		defer stop()
		done := make(chan struct{})
		go func() { io.Copy(channel, local); channel.CloseWrite(); close(done) }()
		io.Copy(local, channel)
		local.(*net.TCPConn).CloseWrite()
		<-done
	}
	peer := &reusePeer{}
	peer.channel = func(ctx context.Context, incoming ssh.NewChannel) bool {
		if incoming.ChannelType() != "direct-tcpip" {
			return false
		}
		var p struct {
			Host       string
			Port       uint32
			Origin     string
			OriginPort uint32
		}
		if ssh.Unmarshal(incoming.ExtraData(), &p) != nil {
			incoming.Reject(ssh.ConnectionFailed, "invalid payload")
			return true
		}
		local, err := net.DialTimeout("tcp", net.JoinHostPort(p.Host, fmt.Sprint(p.Port)), time.Second)
		if err != nil {
			incoming.Reject(ssh.ConnectionFailed, err.Error())
			return true
		}
		channel, requests, err := incoming.Accept()
		if err != nil {
			local.Close()
			return true
		}
		go ssh.DiscardRequests(requests)
		go bridge(ctx, local, channel)
		return true
	}
	peer.requests = func(ctx context.Context, client *ssh.ServerConn, requests <-chan *ssh.Request) {
		listeners := map[string]net.Listener{}
		defer func() {
			for _, l := range listeners {
				l.Close()
			}
		}()
		for request := range requests {
			var p struct {
				Host string
				Port uint32
			}
			ssh.Unmarshal(request.Payload, &p)
			address := net.JoinHostPort(p.Host, fmt.Sprint(p.Port))
			switch request.Type {
			case "tcpip-forward":
				listener, err := net.Listen("tcp", address)
				if err != nil {
					request.Reply(false, nil)
					continue
				}
				listeners[address] = listener
				request.Reply(true, nil)
				go func() {
					for {
						local, err := listener.Accept()
						if err != nil {
							return
						}
						origin := local.RemoteAddr().(*net.TCPAddr)
						payload := struct {
							Host       string
							Port       uint32
							Origin     string
							OriginPort uint32
						}{p.Host, p.Port, origin.IP.String(), uint32(origin.Port)}
						channel, requests, err := client.OpenChannel("forwarded-tcpip", ssh.Marshal(payload))
						if err != nil {
							local.Close()
							continue
						}
						go ssh.DiscardRequests(requests)
						go bridge(ctx, local, channel)
					}
				}()
			case "cancel-tcpip-forward":
				if l := listeners[address]; l != nil {
					l.Close()
					delete(listeners, address)
				}
				request.Reply(true, nil)
			default:
				request.Reply(false, nil)
			}
		}
	}
	return newReusePeer(t, peer)
}

func TestTunnelsReuseSSHAndPreserveHalfClose(t *testing.T) {
	peer := tunnelTestPeer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	remote, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		for {
			conn, err := echo.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				body, _ := io.ReadAll(conn)
				conn.Write(append([]byte("echo:"), body...))
			}()
		}
	}()
	for _, kind := range []string{"local", "remote"} {
		t.Run(kind, func(t *testing.T) {
			rule := TunnelRule{ID: kind, Name: "测试", Type: kind, BindHost: "127.0.0.1", BindPort: tunnelPort(t), TargetHost: "127.0.0.1", TargetPort: echo.Addr().(*net.TCPAddr).Port}
			tunnel, err := remote.StartTunnel(ctx, rule)
			if err != nil {
				t.Fatal(err)
			}
			defer tunnel.Close()
			address := tunnel.Status().Address
			var wg sync.WaitGroup
			for i := range 8 {
				wg.Go(func() {
					conn, err := net.DialTimeout("tcp", address, time.Second)
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					conn.SetDeadline(time.Now().Add(5 * time.Second))
					message := "payload-" + strconv.Itoa(i)
					io.WriteString(conn, message)
					conn.(*net.TCPConn).CloseWrite()
					body, err := io.ReadAll(conn)
					if err != nil || string(body) != "echo:"+message {
						t.Errorf("half-close: %q %v", body, err)
					}
				})
			}
			wg.Wait()
			if peer.connections.Load() != 1 {
				t.Fatal("tunnel opened another SSH login")
			}
			// A conflicting local/remote listener must report a bind failure.
			if duplicate, err := remote.StartTunnel(ctx, rule); err == nil {
				duplicate.Close()
				t.Fatal("duplicate bind accepted")
			}
			tunnel.Close()
			if tunnel.Status().Running {
				t.Fatal("stop left tunnel running")
			}
			deadline := time.Now().Add(2 * time.Second)
			for {
				conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
				if err != nil {
					break
				}
				conn.Close()
				if time.Now().After(deadline) {
					t.Fatal("stop did not release listening port")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}

func TestTunnelErrorsAndDisconnectCleanup(t *testing.T) {
	peer := tunnelTestPeer(t)
	r, err := DialSSH(context.Background(), peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	rule := TunnelRule{ID: "unreachable", Name: "测试", Type: "local", BindHost: "127.0.0.1", BindPort: tunnelPort(t), TargetHost: "127.0.0.1", TargetPort: tunnelPort(t)}
	tunnel, err := r.StartTunnel(context.Background(), rule)
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.Close()
	conn, err := net.DialTimeout("tcp", tunnel.Status().Address, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	io.ReadAll(conn)
	if tunnel.Status().Error == "" {
		t.Fatal("unreachable target was not reported")
	}
	r.Close()
	deadline := time.Now().Add(time.Second)
	for tunnel.Status().Running && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if tunnel.Status().Running {
		t.Fatal("SSH close left local listener running")
	}
	if _, err := r.StartTunnel(context.Background(), rule); err == nil {
		t.Fatal("closed SSH accepted a tunnel")
	}
}

func TestTunnelRuleValidation(t *testing.T) {
	rule := TunnelRule{Name: "有效规则", Type: "local", BindHost: "127.0.0.1", BindPort: 8800, TargetHost: "localhost", TargetPort: 8800}
	if err := rule.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*TunnelRule){
		func(r *TunnelRule) { r.Type = "socks" }, func(r *TunnelRule) { r.BindHost = "localhost" },
		func(r *TunnelRule) { r.BindPort = 0 }, func(r *TunnelRule) { r.TargetPort = 65536 },
		func(r *TunnelRule) { r.TargetHost = "host:22" }, func(r *TunnelRule) { r.TargetHost = "host\nother" },
		func(r *TunnelRule) { r.Name = " " },
	} {
		bad := rule
		mutate(&bad)
		if bad.Validate() == nil {
			t.Fatalf("invalid rule accepted: %+v", bad)
		}
	}
}

func TestTunnelsCloseActiveStreams(t *testing.T) {
	peer := tunnelTestPeer(t)
	target, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer target.Close()
	go func() {
		for {
			conn, err := target.Accept()
			if err != nil {
				return
			}
			go func() { defer conn.Close(); io.WriteString(conn, "ready"); io.Copy(conn, conn) }()
		}
	}()
	for _, kind := range []string{"local", "remote"} {
		for _, stop := range []string{"stop", "cancel", "ssh"} {
			t.Run(kind+"/"+stop, func(t *testing.T) {
				remote, err := DialSSH(context.Background(), peer.server, "test")
				if err != nil {
					t.Fatal(err)
				}
				defer remote.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				rule := TunnelRule{ID: "active", Name: "活动连接", Type: kind, BindHost: "127.0.0.1", BindPort: tunnelPort(t), TargetHost: "127.0.0.1", TargetPort: target.Addr().(*net.TCPAddr).Port}
				tunnel, err := remote.StartTunnel(ctx, rule)
				if err != nil {
					t.Fatal(err)
				}
				defer tunnel.Close()
				conn, err := net.DialTimeout("tcp", tunnel.Status().Address, time.Second)
				if err != nil {
					t.Fatal(err)
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(2 * time.Second))
				if _, err := io.ReadFull(conn, make([]byte, 5)); err != nil {
					t.Fatal("tunnel never connected", err)
				}
				switch stop {
				case "stop":
					tunnel.Close()
				case "cancel":
					cancel()
				case "ssh":
					remote.Close()
				}
				_, err = conn.Read(make([]byte, 1))
				if err == nil {
					t.Fatal("stream stayed open")
				}
				if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
					t.Fatal("cleanup waited for network timeout")
				}
				deadline := time.Now().Add(time.Second)
				for (tunnel.Status().Running || tunnel.Status().Connections != 0) && time.Now().Before(deadline) {
					time.Sleep(time.Millisecond)
				}
				if status := tunnel.Status(); status.Running || status.Connections != 0 {
					t.Fatal("active stream was not released", status)
				}
			})
		}
	}
}
