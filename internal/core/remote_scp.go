package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// SCP writes only the reserved temporary file; Publish owns validation and replacement.
func (r *Remote) uploadSCP(ctx context.Context, server Server, temporary string, source io.Reader, size int64) (int64, error) {
	session, err := r.newSession(ctx)
	if err != nil {
		return 0, err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	stdin, err := session.StdinPipe()
	if err != nil {
		return 0, err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return 0, err
	}
	var stderr strings.Builder
	session.Stderr = &logWriter{write: func(s string) {
		if left := 8192 - stderr.Len(); left > 0 {
			if len(s) > left {
				s = s[:left]
			}
			stderr.WriteString(s)
		}
	}}
	command := "scp -t -- " + Quote(temporary)
	if server.SudoUpload {
		user := server.SudoUser
		if user == "" {
			user = "root"
		}
		command = "sudo -n -u " + Quote(user) + " -- " + command
	}
	if err := session.Start(command); err != nil {
		return 0, fmt.Errorf("SCP 启动失败: %w", err)
	}
	reader := bufio.NewReader(stdout)
	ack := func() error {
		code, err := reader.ReadByte()
		if err != nil {
			return fmt.Errorf("未收到 SCP 确认: %w", err)
		}
		if code == 0 {
			return nil
		}
		message, _ := reader.ReadSlice('\n')
		return fmt.Errorf("SCP 拒绝上传（%d）: %s", code, strings.TrimSpace(string(message)))
	}
	var written int64
	err = func() error {
		if err := ack(); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdin, "C0600 %d upload\n", size); err != nil {
			return err
		}
		if err := ack(); err != nil {
			return err
		}
		var err error
		written, err = io.CopyN(stdin, source, size)
		if err != nil {
			return err
		}
		var extra [1]byte
		if n, err := source.Read(extra[:]); n != 0 || err != io.EOF {
			return fmt.Errorf("本地文件大小发生变化: %w", errors.Join(err, io.ErrUnexpectedEOF))
		}
		if _, err := stdin.Write([]byte{0}); err != nil {
			return err
		}
		return ack()
	}()
	stdin.Close()
	if err != nil {
		session.Close()
	}
	err = errors.Join(err, session.Wait(), ctx.Err())
	if err != nil {
		return written, fmt.Errorf("SCP 上传失败: %w %s", err, strings.TrimSpace(stderr.String()))
	}
	return written, nil
}
