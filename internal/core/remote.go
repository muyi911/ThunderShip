package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type UnknownError struct{ Err error }

func (e *UnknownError) Error() string { return "远端结果未知: " + e.Err.Error() }
func (e *UnknownError) Unwrap() error { return e.Err }

type HostKeyError struct {
	Fingerprint string
	Changed     bool
}

func (e *HostKeyError) Error() string {
	if e.Changed {
		return "服务器指纹已变化，连接已阻止: " + e.Fingerprint
	}
	return "首次连接需要确认指纹: " + e.Fingerprint
}

type Remote struct {
	SSH     *ssh.Client
	SFTP    *sftp.Client
	session *ssh.Session
	parent  *Remote
	server  Server
	closed  chan struct{}
	once    sync.Once
}

func Dial(ctx context.Context, s Server, secret string, probe bool) (*Remote, error) {
	r, err := DialSSH(ctx, s, secret)
	if err != nil {
		return nil, err
	}
	r.server = s
	if err = r.initSFTP(ctx, s, probe); err != nil {
		r.Close()
		return nil, err
	}
	return r, nil
}

// DialSSH opens the run's transport; upload channels are created separately.
func DialSSH(ctx context.Context, s Server, secret string) (*Remote, error) {
	var auth ssh.AuthMethod
	if s.Auth == "key" {
		b, e := os.ReadFile(s.KeyPath)
		if e != nil {
			return nil, e
		}
		var signer ssh.Signer
		if secret != "" {
			signer, e = ssh.ParsePrivateKeyWithPassphrase(b, []byte(secret))
		} else {
			signer, e = ssh.ParsePrivateKey(b)
		}
		if e != nil {
			return nil, fmt.Errorf("无法读取私钥或口令错误")
		}
		auth = ssh.PublicKeys(signer)
	} else {
		auth = ssh.Password(secret)
	}
	cfg := &ssh.ClientConfig{User: s.User, Auth: []ssh.AuthMethod{auth}, Timeout: 15 * time.Second, HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
		fp := ssh.FingerprintSHA256(key)
		if fp != s.Fingerprint {
			return &HostKeyError{Fingerprint: fp, Changed: s.Fingerprint != ""}
		}
		return nil
	}}
	addr := net.JoinHostPort(s.Host, fmt.Sprint(s.Port))
	conn, e := (&net.Dialer{Timeout: 15 * time.Second}).DialContext(ctx, "tcp", addr)
	if e != nil {
		return nil, e
	}
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	handshakeDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-handshakeDone:
		}
	}()
	c, ch, req, e := ssh.NewClientConn(conn, addr, cfg)
	close(handshakeDone)
	if e != nil {
		conn.Close()
		return nil, e
	}
	_ = conn.SetDeadline(time.Time{})
	client := ssh.NewClient(c, ch, req)
	r := &Remote{SSH: client, closed: make(chan struct{})}
	go func() {
		_ = client.Wait()
		r.Close()
	}()
	go func() {
		select {
		case <-ctx.Done():
			r.Close()
		case <-r.closed:
		}
	}()
	// Bound a silent peer as well as context cancellation; keepalives share the transport deadline.
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-r.closed:
				return
			case <-t.C:
				_ = conn.SetDeadline(time.Now().Add(45 * time.Second))
				if _, _, e := client.SendRequest("keepalive@openssh.com", true, nil); e != nil {
					r.Close()
					return
				}
			}
		}
	}()
	return r, nil
}

// Each transfer owns a channel, so its timeout cannot close other nodes' transport.
func (r *Remote) OpenTransfer(ctx context.Context, s Server) (Session, error) {
	session, err := r.newSession(ctx)
	if err != nil {
		return nil, err
	}
	child := &Remote{SSH: r.SSH, session: session, parent: r, server: s, closed: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			child.Close()
		case <-child.closed:
		}
	}()
	if err := child.initSFTP(ctx, s, false); err != nil {
		child.Close()
		return nil, err
	}
	return child, nil
}

func (r *Remote) initSFTP(ctx context.Context, s Server, probe bool) error {
	if r.session == nil {
		var err error
		r.session, err = r.newSession(ctx)
		if err != nil {
			return err
		}
	}
	session := r.session
	in, e := session.StdinPipe()
	if e != nil {
		return e
	}
	out, e := session.StdoutPipe()
	if e != nil {
		return e
	}
	session.Stderr = io.Discard
	if s.SudoUpload {
		user := s.SudoUser
		if user == "" {
			user = "root"
		}
		if !path.IsAbs(s.SFTPPath) {
			return fmt.Errorf("请填写 sudo sftp-server 的绝对路径")
		}
		if e = session.Start("sudo -n -u " + Quote(user) + " -- " + Quote(s.SFTPPath)); e != nil {
			return e
		}
	} else {
		if e = session.RequestSubsystem("sftp"); e != nil {
			return e
		}
	}
	r.SFTP, e = sftp.NewClientPipe(out, in)
	if e != nil {
		return fmt.Errorf("SFTP 启动失败，请检查服务及上传权限: %w", e)
	}
	if _, ok := r.SFTP.HasExtension("posix-rename@openssh.com"); !ok && !probe {
		return fmt.Errorf("服务器不支持原子替换扩展 posix-rename@openssh.com")
	}
	return nil
}
func (r *Remote) Close() {
	r.once.Do(func() {
		close(r.closed)
		if r.parent != nil {
			r.session.Close()
		} else {
			r.SSH.Close()
		}
	})
}

func (r *Remote) isClosed() bool {
	select {
	case <-r.closed:
		return true
	default:
		return false
	}
}

func (r *Remote) newSession(ctx context.Context) (*ssh.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		session *ssh.Session
		err     error
	}
	for {
		opened := make(chan result)
		go func() {
			session, err := r.SSH.NewSession()
			select {
			case opened <- result{session, err}:
			case <-ctx.Done():
				if session != nil {
					session.Close()
				}
			}
		}()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case got := <-opened:
			var shortage *ssh.OpenChannelError
			if !errors.As(got.err, &shortage) || (shortage.Reason != ssh.ResourceShortage && shortage.Reason != ssh.ConnectionFailed) {
				return got.session, got.err
			}
		}
		// OpenSSH reports MaxSessions as ConnectionFailed; retry only before any command is sent.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Run adapts Spot's sshRun context/signal pattern. See THIRD_PARTY_NOTICES.md.
func (r *Remote) Run(ctx context.Context, s Step, output func(string)) error {
	session, e := r.newSession(ctx)
	if e != nil {
		return e
	}
	defer session.Close()
	stopCancellation := context.AfterFunc(ctx, func() { session.Close() })
	defer stopCancellation()
	cmd := s.Command
	if s.Directory != "" {
		cmd = "cd -- " + Quote(s.Directory) + " || exit $?\n" + cmd
	}
	cmd = "sh -c " + Quote(cmd)
	if s.Sudo {
		cmd = "sudo -n -- " + cmd
	}
	w := &logWriter{write: output}
	session.Stdout = w
	session.Stderr = w
	if e = session.Start(cmd); e != nil {
		if ctx.Err() != nil {
			return &UnknownError{ctx.Err()}
		}
		return e
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case e = <-done:
		if e == nil {
			return nil
		}
		var exit *ssh.ExitError
		if errors.As(e, &exit) {
			return fmt.Errorf("命令退出码 %d", exit.ExitStatus())
		}
		return &UnknownError{e}
	case <-ctx.Done():
		return &UnknownError{ctx.Err()}
	}
}

type logWriter struct {
	mu    sync.Mutex
	write func(string)
}

func (w *logWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.write != nil {
		w.write(string(b))
	}
	return len(b), nil
}

type progressReader struct {
	ctx      context.Context
	r        io.Reader
	n        int64
	last     time.Time
	progress func(int64)
}

func (r *progressReader) Read(b []byte) (int, error) {
	if e := r.ctx.Err(); e != nil {
		return 0, e
	}
	n, e := r.r.Read(b)
	r.n += int64(n)
	if r.progress != nil && (r.last.IsZero() || time.Since(r.last) >= 150*time.Millisecond || e != nil) {
		r.last = time.Now()
		r.progress(r.n)
	}
	return n, e
}

func (r *Remote) CheckFiles(files []Artifact) error {
	if _, ok := r.SFTP.HasExtension("posix-rename@openssh.com"); !ok {
		return fmt.Errorf("缺少原子重命名扩展")
	}
	for _, f := range files {
		if e := r.noLinks(f.Remote); e != nil {
			return e
		}
		info, e := r.SFTP.Lstat(f.Remote)
		if e == nil && !info.Mode().IsRegular() {
			return fmt.Errorf("目标不是普通文件: %s", f.Remote)
		}
		if e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
func (r *Remote) noLinks(p string) error {
	cur := "/"
	for _, part := range strings.Split(strings.TrimPrefix(path.Clean(p), "/"), "/") {
		cur = path.Join(cur, part)
		i, e := r.SFTP.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		if i.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("不允许符号链接路径: %s", cur)
		}
	}
	return nil
}
func (r *Remote) Publish(ctx context.Context, files []Artifact, before Step, report func(string, int64, int64, string)) error {
	if before.UploadProtocol != "" && before.UploadProtocol != "sftp" && before.UploadProtocol != "scp" {
		return fmt.Errorf("上传方式须为 SFTP 或 SCP")
	}
	original := r
	stopCancellation := context.AfterFunc(ctx, original.Close)
	defer stopCancellation()
	if e := r.CheckFiles(files); e != nil {
		return e
	}
	type prepared struct{ tmp, remote string }
	var ready []prepared
	defer func() {
		for _, f := range ready {
			if f.tmp != "" {
				_ = r.SFTP.Remove(f.tmp)
			}
		}
		if r != original {
			r.Close()
		}
	}()
	for _, f := range files {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := r.SFTP.MkdirAll(path.Dir(f.Remote)); e != nil {
			return e
		}
		if e := r.noLinks(f.Remote); e != nil {
			return e
		}
		tmp := path.Join(path.Dir(f.Remote), "."+path.Base(f.Remote)+"."+ID()+".part")
		out, e := r.SFTP.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
		if e != nil {
			return e
		}
		ready = append(ready, prepared{tmp, f.Remote})
		if e = out.Chmod(0600); e != nil {
			out.Close()
			return e
		}
		in, e := os.Open(f.Local)
		if e != nil {
			out.Close()
			return e
		}
		reader := &progressReader{ctx: ctx, r: in, progress: func(n int64) { report(f.Remote, n, f.Size, "uploading") }}
		var written int64
		var closeErr error
		if before.UploadProtocol == "scp" {
			closeErr = out.Close()
			if closeErr == nil {
				server := r.server
				runner := r
				// Release SFTP while SCP runs so MaxSessions=1 still works.
				if r.parent != nil {
					runner = r.parent
					r.Close()
				} else {
					r.SFTP.Close()
					r.session.Close()
				}
				written, e = runner.uploadSCP(ctx, server, tmp, reader, f.Size)
				opened, reopenErr := runner.OpenTransfer(ctx, server)
				if reopenErr == nil {
					r = opened.(*Remote)
				}
				e = errors.Join(e, reopenErr)
			}
		} else {
			// The progress wrapper hides the file size from ReadFrom.
			written, e = out.ReadFromWithConcurrency(reader, 0)
			closeErr = out.Close()
		}
		in.Close()
		if e != nil {
			return e
		}
		if closeErr != nil {
			return closeErr
		}
		info, e := r.SFTP.Stat(tmp)
		if e != nil {
			return e
		}
		if written != f.Size || info.Size() != f.Size {
			return fmt.Errorf("上传文件大小不一致: %s（预期 %d，已传输 %d，远端 %d）", f.Remote, f.Size, written, info.Size())
		}
		mode := os.FileMode(0644)
		if f.Executable {
			mode = 0755
		}
		old, e := r.SFTP.Lstat(f.Remote)
		if e == nil {
			mode = old.Mode().Perm()
			if f.Executable {
				mode |= 0111
			}
			if st, ok := old.Sys().(*sftp.FileStat); ok {
				if e = r.SFTP.Chown(tmp, int(st.UID), int(st.GID)); e != nil {
					return fmt.Errorf("无法保留属主/属组 %s: %w", f.Remote, e)
				}
			}
		} else if !os.IsNotExist(e) {
			return e
		}
		if e = r.SFTP.Chmod(tmp, mode); e != nil {
			return e
		}
		report(f.Remote, f.Size, f.Size, "prepared")
	}
	if before.Command != "" {
		runner := r
		if r.parent != nil {
			// Release SFTP first so a server with MaxSessions=1 can run the command.
			runner = r.parent
			r.Close()
		}
		commandErr := runner.Run(ctx, before, func(s string) { report(s, 0, 0, "log") })
		if r.parent != nil {
			opened, err := runner.OpenTransfer(ctx, r.server)
			if err != nil {
				return errors.Join(commandErr, err)
			}
			r = opened.(*Remote)
		}
		if commandErr != nil {
			return commandErr
		}
	}
	for i, f := range ready {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := r.noLinks(f.remote); e != nil {
			return e
		}
		if e := r.SFTP.PosixRename(f.tmp, f.remote); e != nil {
			return &UnknownError{fmt.Errorf("发布 %s 未确认，核查文件后重试: %w", f.remote, e)}
		}
		ready[i].tmp = ""
		report(f.remote, 0, 0, "published")
	}
	return nil
}
