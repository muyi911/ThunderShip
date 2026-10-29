package core

import (
	"context"
	"io"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type fileActionRecorder struct {
	sftp.FileCmder
	mode atomic.Uint32
}

func (c *fileActionRecorder) Filecmd(req *sftp.Request) error {
	if req.Method == "Setstat" && req.AttrFlags().Permissions {
		c.mode.Store(req.Attributes().Mode)
	}
	return c.FileCmder.Filecmd(req)
}

func TestRemoteFileActionsPreserveExistingFilesAndLinks(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	commands := &fileActionRecorder{FileCmder: peer.files.FileCmd}
	peer.files.FileCmd = commands
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := Dial(ctx, peer.server, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	apply := func(action, source, target, mode string) error {
		return r.ManageFile(ctx, RemoteFileOperation{Action: action, Source: source, Target: target, Mode: mode}, "")
	}
	if err := apply("mkdir", "/folder", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply("create", "/folder/config.txt", "", ""); err != nil {
		t.Fatal(err)
	}
	f, err := r.SFTP.OpenFile("/folder/config.txt", os.O_WRONLY)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(f, "keep original")
	f.Close()
	if err := apply("create", "/folder/config.txt", "", ""); err == nil {
		t.Fatal("new file truncated an existing file")
	}
	if err := apply("rename", "/folder/config.txt", "/folder/renamed.txt", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply("create", "/folder/existing.txt", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := apply("rename", "/folder/renamed.txt", "/folder/existing.txt", ""); err == nil {
		t.Fatal("rename accepted an existing destination")
	}
	if err := apply("chmod", "/folder/renamed.txt", "", "4750"); err != nil {
		t.Fatal(err)
	}
	if got := commands.mode.Load() & 07777; got != 04750 {
		t.Fatalf("chmod lost Unix special bits: %o", got)
	}
	if err := r.SFTP.Symlink("/folder", "/linked"); err != nil {
		t.Fatal(err)
	}
	if err := apply("create", "/linked/escape", "", ""); err == nil {
		t.Fatal("write through a symlinked parent accepted")
	}
	if err := apply("chmod", "/linked", "", "777"); err == nil {
		t.Fatal("chmod through a symlink accepted")
	}
	if err := apply("delete", "/linked", "", ""); err != nil {
		t.Fatal(err)
	}
	text, err := r.ReadText("/folder/renamed.txt")
	if err != nil || text.Content != "keep original" {
		t.Fatal("existing file or symlink target changed", text, err)
	}
	if err := apply("move", "/folder/renamed.txt", "/moved.txt", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := r.SFTP.Lstat("/folder/renamed.txt"); !os.IsNotExist(err) {
		t.Fatal("move left the source", err)
	}
	if err := apply("delete", "/moved.txt", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteFileCopyStagesServerSideAndDoesNotOverwrite(t *testing.T) {
	var remote *Remote
	const source = "/original 'quoted; $(touch should-not-run).txt"
	preparedPattern := regexp.MustCompile(`/dest/\.thundership-copy-[a-f0-9]+/item`)
	stagePattern := regexp.MustCompile(`/dest/\.thundership-copy-[a-f0-9]+`)
	var copies atomic.Int32
	var createCollision atomic.Bool
	peer := newReusePeer(t, &reusePeer{exec: func(_ context.Context, channel ssh.Channel, command string) {
		if strings.Contains(command, "cp -a --") {
			prepared := preparedPattern.FindString(command)
			if prepared == "" || command != "sh -c "+Quote("cp -a -- "+Quote(source)+" "+Quote(prepared)) {
				t.Error("copy was not a fully quoted server-side command", command)
				return
			}
			copies.Add(1)
			f, err := remote.SFTP.Create(prepared)
			if err != nil {
				t.Error(err)
				return
			}
			io.WriteString(f, "server copied content")
			f.Close()
			if createCollision.Load() {
				f, err := remote.SFTP.Create("/dest/racing.txt")
				if err != nil {
					t.Error(err)
					return
				}
				io.WriteString(f, "external writer")
				f.Close()
			}
		} else if strings.Contains(command, "rm -rf --") {
			stage := stagePattern.FindString(command)
			_ = remote.SFTP.Remove(stage + "/item")
			if err := remote.SFTP.RemoveDirectory(stage); err != nil {
				t.Error("staging cleanup failed", err)
			}
		}
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var err error
	remote, err = Dial(ctx, peer.server, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer remote.Close()
	remote.SFTP.Mkdir("/dest")
	f, err := remote.SFTP.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(f, "source content")
	f.Close()
	op := RemoteFileOperation{Action: "copy", Source: source, Target: "/dest/copy.txt"}
	if err := remote.ManageFile(ctx, op, ""); err != nil {
		t.Fatal(err)
	}
	if err := remote.ManageFile(ctx, op, ""); err == nil {
		t.Fatal("copy accepted an existing destination")
	}
	text, err := remote.ReadText(op.Target)
	if err != nil || text.Content != "server copied content" || copies.Load() != 1 {
		t.Fatal("copy result", text, err, copies.Load())
	}
	text, err = remote.ReadText(source)
	if err != nil || text.Content != "source content" {
		t.Fatal("copy changed the original", text, err)
	}
	entries, err := remote.SFTP.ReadDir("/dest")
	if err != nil || len(entries) != 1 || entries[0].Name() != "copy.txt" {
		t.Fatal("staging directory leaked", entries, err)
	}
	createCollision.Store(true)
	op.Target = "/dest/racing.txt"
	if err := remote.ManageFile(ctx, op, ""); err == nil {
		t.Fatal("destination created during the copy was overwritten")
	}
	text, err = remote.ReadText(op.Target)
	if err != nil || text.Content != "external writer" {
		t.Fatal("copy replaced another writer's file", text, err)
	}
	entries, err = remote.SFTP.ReadDir("/dest")
	if err != nil || len(entries) != 2 {
		t.Fatal("failed copy leaked its staging directory", entries, err)
	}
}

func TestRemoteFileOperationValidation(t *testing.T) {
	for _, op := range []RemoteFileOperation{
		{Action: "delete", Source: "/"}, {Action: "create", Source: "relative"},
		{Action: "mkdir", Source: "/a/../b"}, {Action: "delete", Source: "/a\nfile"},
		{Action: "copy", Source: "/dir", Target: "/dir/child"}, {Action: "move", Source: "/a", Target: "/a"},
		{Action: "rename", Source: "/a/file", Target: "/b/file"}, {Action: "chmod", Source: "/a", Mode: "888"},
		{Action: "chmod", Source: "/a", Mode: "07555"}, {Action: "execute", Source: "/a"},
	} {
		if err := op.Validate(); err == nil {
			t.Fatal("invalid operation accepted", op)
		}
	}
}
