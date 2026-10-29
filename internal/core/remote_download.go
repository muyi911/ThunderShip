package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

func (r *Remote) Download(ctx context.Context, remote string, out io.Writer, protocol string) error {
	if protocol != "sftp" && protocol != "scp" {
		return fmt.Errorf("下载方式须为 SFTP 或 SCP")
	}
	if err := r.noLinks(remote); err != nil {
		return err
	}
	info, err := r.SFTP.Stat(remote)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("下载仅支持普通文件")
	}
	if protocol == "sftp" {
		file, err := r.SFTP.Open(remote)
		if err != nil {
			return err
		}
		defer file.Close()
		n, err := io.Copy(out, file)
		if err != nil {
			return err
		}
		if n != info.Size() {
			return fmt.Errorf("下载文件大小不一致")
		}
		return nil
	}
	r.SFTP.Close()
	r.session.Close()
	session, err := r.newSession(ctx)
	if err != nil {
		return err
	}
	defer session.Close()
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	session.Stderr = &logWriter{write: func(text string) {
		if left := 8192 - stderr.Len(); left > 0 {
			stderr.WriteString(text[:min(len(text), left)])
		}
	}}
	command := "scp -f -- " + Quote(remote)
	if r.server.SudoUpload {
		user := r.server.SudoUser
		if user == "" {
			user = "root"
		}
		command = "sudo -n -u " + Quote(user) + " -- " + command
	}
	if err := session.Start(command); err != nil {
		return err
	}
	reader := bufio.NewReader(stdout)
	err = func() error {
		if _, err := stdin.Write([]byte{0}); err != nil {
			return err
		}
		header, err := reader.ReadSlice('\n')
		if err != nil {
			return err
		}
		var mode, name string
		var size int64
		if _, err := fmt.Sscanf(string(header), "C%s %d %s", &mode, &size, &name); err != nil || size != info.Size() {
			return fmt.Errorf("SCP 文件头无效或文件大小发生变化: %q", header)
		}
		if _, err := stdin.Write([]byte{0}); err != nil {
			return err
		}
		if _, err := io.CopyN(out, reader, size); err != nil {
			return err
		}
		if code, err := reader.ReadByte(); err != nil || code != 0 {
			return fmt.Errorf("SCP 未确认下载成功: %d %v", code, err)
		}
		_, err = stdin.Write([]byte{0})
		return err
	}()
	stdin.Close()
	if err != nil {
		session.Close()
	}
	err = errors.Join(err, session.Wait(), ctx.Err())
	if err != nil {
		return fmt.Errorf("SCP 下载失败: %w %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}
