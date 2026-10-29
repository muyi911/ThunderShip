package core

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

type textFileCommands struct {
	sftp.FileCmder
	denyRename atomic.Bool
}

func (c *textFileCommands) Filecmd(req *sftp.Request) error {
	if c.denyRename.Load() && (req.Method == "Rename" || req.Method == "PosixRename") {
		return os.ErrPermission
	}
	return c.FileCmder.Filecmd(req)
}
func (c *textFileCommands) PosixRename(req *sftp.Request) error {
	if c.denyRename.Load() {
		return os.ErrPermission
	}
	return c.FileCmder.(sftp.PosixRenameFileCmder).PosixRename(req)
}

func TestRemoteTextSavePreservesFileAndRejectsStaleOrInvalidDrafts(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	commands := &textFileCommands{FileCmder: peer.files.FileCmd}
	peer.files.FileCmd = commands
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := Dial(ctx, peer.server, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	write := func(name, text string) {
		t.Helper()
		f, err := r.SFTP.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, err = io.WriteString(f, text)
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			t.Fatal(err, closeErr)
		}
	}
	read := func(name string) RemoteText {
		t.Helper()
		text, err := r.ReadText(name)
		if err != nil {
			t.Fatal(err)
		}
		return text
	}
	const name = "/config.env"
	write(name, "\ufeffNAME=测试\r\nPORT=80\r\n")
	if err := r.SFTP.Chmod(name, 0640); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := r.SFTP.Stat(name)
	if err != nil {
		t.Fatal(err)
	}
	before := read(name)
	after, err := r.SaveText(name, "NAME=修改😀\nPORT=81\n", before.Version)
	if err != nil {
		t.Fatal(err)
	}
	if after.Content != "\ufeffNAME=修改😀\r\nPORT=81\r\n" || read(name) != after || before.Version == after.Version {
		t.Fatal("saved text, BOM, CRLF or revision lost", after)
	}
	info, err := r.SFTP.Stat(name)
	if err != nil || info.Mode().Perm() != originalInfo.Mode().Perm() {
		t.Fatal("permissions lost", info, err)
	}
	if _, err = r.SaveText(name, "overwrite", before.Version); err == nil || !strings.Contains(err.Error(), "已被修改") {
		t.Fatal("stale editor overwrote server changes", err)
	}
	if read(name) != after {
		t.Fatal("conflict modified remote file")
	}
	for _, content := range []string{strings.Repeat("x", MaxRemoteText+1), "binary\x00data", "invalid\xff"} {
		if _, err = r.SaveText(name, content, after.Version); err == nil {
			t.Fatal("invalid draft accepted")
		}
		if read(name) != after {
			t.Fatal("invalid draft changed original")
		}
	}
	commands.denyRename.Store(true)
	if _, err = r.SaveText(name, "must fail", after.Version); err == nil {
		t.Fatal("rename failure reported success")
	} else {
		var unknown *UnknownError
		if !errors.As(err, &unknown) {
			t.Fatal("unconfirmed save was not marked unknown", err)
		}
	}
	commands.denyRename.Store(false)
	if read(name) != after {
		t.Fatal("failed save destroyed original")
	}
	entries, err := r.SFTP.ReadDir("/")
	if err != nil || len(entries) != 1 {
		t.Fatal("temporary file leaked", entries, err)
	}
	write("/empty", "")
	empty := read("/empty")
	if empty.Content != "" || empty.Version == "" {
		t.Fatal("empty file cannot be edited")
	}
	if _, err = r.SaveText("/empty", "", empty.Version); err != nil {
		t.Fatal(err)
	}
	write("/binary", "\x00binary")
	write("/large", strings.Repeat("x", MaxRemoteText+1))
	if err := r.SFTP.Symlink(name, "/link"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/binary", "/large", "/link", "/", "/missing"} {
		if _, err = r.ReadText(path); err == nil {
			t.Fatal("invalid editable file accepted", path)
		}
	}
	if _, err = r.SaveText("/missing", "new file", "anything"); err == nil {
		t.Fatal("missing file was silently recreated")
	}
}
