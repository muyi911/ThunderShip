package core

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/pkg/sftp"
)

const MaxRemoteText = 2 * 1024 * 1024

type RemoteText struct {
	Content string `json:"content"`
	Version string `json:"version"`
}

func validRemoteText(content string) error {
	if len(content) > MaxRemoteText {
		return fmt.Errorf("在线编辑仅支持 2 MB 以内的文本文件，请下载后编辑")
	}
	if !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return fmt.Errorf("文件不是 UTF-8 文本，请下载后使用合适的编辑器打开")
	}
	return nil
}

func (r *Remote) ReadText(remote string) (RemoteText, error) {
	if err := r.noLinks(remote); err != nil {
		return RemoteText{}, err
	}
	info, err := r.SFTP.Lstat(remote)
	if err != nil {
		return RemoteText{}, err
	}
	if !info.Mode().IsRegular() {
		return RemoteText{}, fmt.Errorf("在线编辑仅支持普通文本文件")
	}
	in, err := r.SFTP.Open(remote)
	if err != nil {
		return RemoteText{}, err
	}
	defer in.Close()
	info, err = in.Stat()
	if err != nil {
		return RemoteText{}, err
	}
	if !info.Mode().IsRegular() {
		return RemoteText{}, fmt.Errorf("在线编辑仅支持普通文本文件")
	}
	if info.Size() > MaxRemoteText {
		return RemoteText{}, fmt.Errorf("在线编辑仅支持 2 MB 以内的文本文件，请下载后编辑")
	}
	data, err := io.ReadAll(io.LimitReader(in, MaxRemoteText+1))
	if err != nil {
		return RemoteText{}, err
	}
	content := string(data)
	if err := validRemoteText(content); err != nil {
		return RemoteText{}, err
	}
	return RemoteText{content, fmt.Sprintf("%x", sha256.Sum256(data))}, nil
}

func (r *Remote) SaveText(remote, content, version string) (RemoteText, error) {
	if err := validRemoteText(content); err != nil {
		return RemoteText{}, err
	}
	current, err := r.ReadText(remote)
	if err != nil {
		return RemoteText{}, err
	}
	if version == "" || current.Version != version {
		return RemoteText{}, fmt.Errorf("服务器文件已被修改，请保留草稿并重新打开文件后再保存")
	}
	// Textareas normalize newlines; retain the original file's CRLF and UTF-8 BOM.
	if strings.Contains(current.Content, "\r\n") && !strings.Contains(strings.ReplaceAll(current.Content, "\r\n", ""), "\n") {
		content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "\r\n")
	}
	if strings.HasPrefix(current.Content, "\ufeff") && !strings.HasPrefix(content, "\ufeff") {
		content = "\ufeff" + content
	}
	if err := validRemoteText(content); err != nil {
		return RemoteText{}, err
	}
	if err := r.CheckFiles([]Artifact{{Remote: remote}}); err != nil {
		return RemoteText{}, err
	}
	info, err := r.SFTP.Lstat(remote)
	if err != nil {
		return RemoteText{}, err
	}
	tmp := path.Join(path.Dir(remote), "."+path.Base(remote)+"."+ID()+".part")
	out, err := r.SFTP.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return RemoteText{}, err
	}
	defer r.SFTP.Remove(tmp)
	if err = out.Chmod(0600); err == nil {
		_, err = io.WriteString(out, content)
	}
	closeErr := out.Close()
	if err != nil {
		return RemoteText{}, err
	}
	if closeErr != nil {
		return RemoteText{}, closeErr
	}
	prepared, err := r.ReadText(tmp)
	if err != nil {
		return RemoteText{}, err
	}
	if prepared.Content != content {
		return RemoteText{}, fmt.Errorf("文件内容校验失败，原文件未修改")
	}
	if stat, ok := info.Sys().(*sftp.FileStat); ok {
		if err = r.SFTP.Chown(tmp, int(stat.UID), int(stat.GID)); err != nil {
			return RemoteText{}, fmt.Errorf("无法保留文件属主/属组：%w", err)
		}
	}
	if err = r.SFTP.Chmod(tmp, info.Mode()); err != nil {
		return RemoteText{}, err
	}
	latest, err := r.ReadText(remote)
	if err != nil {
		return RemoteText{}, err
	}
	if latest.Version != version {
		return RemoteText{}, fmt.Errorf("服务器文件已被修改，请保留草稿并重新打开文件后再保存")
	}
	// shortcut: SFTP has no compare-and-swap; external writers can race this last check.
	if err = r.SFTP.PosixRename(tmp, remote); err != nil {
		return RemoteText{}, &UnknownError{fmt.Errorf("保存 %s 未确认，请核查服务器文件后重试：%w", remote, err)}
	}
	return prepared, nil
}
