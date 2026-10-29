package core

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

type TunnelRule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	BindHost   string `json:"bindHost"`
	BindPort   int    `json:"bindPort"`
	TargetHost string `json:"targetHost"`
	TargetPort int    `json:"targetPort"`
}

func (r TunnelRule) Validate() error {
	if strings.TrimSpace(r.Name) == "" || len(r.Name) > 200 {
		return fmt.Errorf("请填写隧道名称（最多 200 字节）")
	}
	if r.Type != "local" && r.Type != "remote" {
		return fmt.Errorf("请选择本地或远程 TCP 转发")
	}
	if net.ParseIP(r.BindHost) == nil {
		return fmt.Errorf("监听地址必须是 IP 地址，例如 127.0.0.1")
	}
	if r.TargetHost == "" || len(r.TargetHost) > 253 || strings.ContainsAny(r.TargetHost, " \t\r\n\x00/\\[]") {
		return fmt.Errorf("请填写有效的目标主机名或 IP 地址")
	}
	if strings.Contains(r.TargetHost, ":") && net.ParseIP(r.TargetHost) == nil {
		return fmt.Errorf("目标端口请单独填写")
	}
	if r.BindPort < 1 || r.BindPort > 65535 || r.TargetPort < 1 || r.TargetPort > 65535 {
		return fmt.Errorf("端口必须是 1–65535 的整数")
	}
	return nil
}

type TunnelStatus struct {
	ID          string `json:"id"`
	Running     bool   `json:"running"`
	Address     string `json:"address"`
	Connections int    `json:"connections"`
	Error       string `json:"error"`
}

type Tunnel struct {
	mu       sync.Mutex
	listener net.Listener
	rule     TunnelRule
	ctx      context.Context
	cancel   context.CancelFunc
	status   TunnelStatus
	peers    map[net.Conn]bool
}

// StartTunnel opens channels on the existing SSH transport, without another login.
func (r *Remote) StartTunnel(ctx context.Context, rule TunnelRule) (*Tunnel, error) {
	client := r.SSH
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	if r.isClosed() {
		return nil, fmt.Errorf("SSH 连接已关闭")
	}
	address := net.JoinHostPort(rule.BindHost, strconv.Itoa(rule.BindPort))
	var listener net.Listener
	var err error
	if rule.Type == "local" {
		listener, err = (&net.ListenConfig{}).Listen(ctx, "tcp", address)
	} else {
		// SSH global requests have no context API; dispose a late reply after timeout.
		listenCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		listener, err = listenRemote(listenCtx, client, address)
	}
	if err != nil {
		return nil, fmt.Errorf("无法启动隧道（端口被占用或服务器禁止转发）: %w", err)
	}
	if r.isClosed() || ctx.Err() != nil {
		listener.Close()
		return nil, fmt.Errorf("SSH 连接已关闭或隧道启动已取消")
	}
	ctx, cancel := context.WithCancel(ctx)
	t := &Tunnel{listener: listener, rule: rule, ctx: ctx, cancel: cancel,
		status: TunnelStatus{ID: rule.ID, Running: true, Address: listener.Addr().String()}, peers: map[net.Conn]bool{}}
	context.AfterFunc(ctx, t.Close)
	go func() {
		select {
		case <-r.closed:
			t.Close()
		case <-ctx.Done():
		}
	}()
	go t.serve(client)
	return t, nil
}

func listenRemote(ctx context.Context, client *ssh.Client, address string) (net.Listener, error) {
	type result struct {
		listener net.Listener
		err      error
	}
	ch := make(chan result)
	go func() {
		l, err := client.Listen("tcp", address)
		select {
		case ch <- result{l, err}:
		case <-ctx.Done():
			if l != nil {
				l.Close()
			}
		}
	}()
	select {
	case r := <-ch:
		return r.listener, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *Tunnel) Status() TunnelStatus {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.status
}

func (t *Tunnel) Close() {
	t.mu.Lock()
	if !t.status.Running {
		t.mu.Unlock()
		return
	}
	t.status.Running = false
	peers := make([]net.Conn, 0, len(t.peers))
	for conn := range t.peers {
		peers = append(peers, conn)
	}
	t.mu.Unlock()
	t.cancel()
	for _, conn := range peers {
		conn.Close()
	}
	if t.rule.Type == "remote" {
		go t.listener.Close() // A remote cancel request must not hold up the UI.
	} else {
		t.listener.Close()
	}
}

func (t *Tunnel) fail(err error) {
	t.mu.Lock()
	if t.status.Running {
		t.status.Error = err.Error()
	}
	t.mu.Unlock()
}

func (t *Tunnel) serve(client *ssh.Client) {
	for {
		incoming, err := t.listener.Accept()
		if err != nil {
			t.fail(err)
			t.Close()
			return
		}
		t.mu.Lock()
		if !t.status.Running {
			t.mu.Unlock()
			incoming.Close()
			return
		}
		t.peers[incoming] = true
		t.status.Connections++
		t.mu.Unlock()
		go t.forward(client, incoming)
	}
}

func (t *Tunnel) forward(client *ssh.Client, incoming net.Conn) {
	defer func() {
		incoming.Close()
		t.mu.Lock()
		delete(t.peers, incoming)
		t.status.Connections--
		t.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(t.ctx, 15*time.Second)
	address := net.JoinHostPort(t.rule.TargetHost, strconv.Itoa(t.rule.TargetPort))
	var outgoing net.Conn
	var err error
	if t.rule.Type == "local" {
		outgoing, err = client.DialContext(ctx, "tcp", address)
	} else {
		outgoing, err = (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
	cancel()
	if err != nil {
		t.fail(err)
		return
	}
	defer outgoing.Close()
	t.mu.Lock()
	if !t.status.Running {
		t.mu.Unlock()
		return
	}
	t.peers[outgoing] = true
	t.status.Error = ""
	t.mu.Unlock()
	defer func() { t.mu.Lock(); delete(t.peers, outgoing); t.mu.Unlock() }()
	done := make(chan struct{})
	copyStream := func(dst, src net.Conn) {
		if _, err := io.Copy(dst, src); err != nil {
			if t.ctx.Err() == nil {
				t.fail(err)
			}
			dst.Close()
			src.Close()
			return
		}
		if half, ok := dst.(interface{ CloseWrite() error }); ok {
			half.CloseWrite()
		} else {
			dst.Close()
		}
	}
	go func() { copyStream(outgoing, incoming); close(done) }()
	copyStream(incoming, outgoing)
	<-done
}
