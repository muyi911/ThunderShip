package core

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type ownerTestEntry struct {
	os.FileInfo
	uid, gid uint32
}

func (entry ownerTestEntry) Sys() any { return &sftp.FileStat{UID: entry.uid, GID: entry.gid} }

func TestRemoteOwnerNamesResolveNSSNamesAndFallbackPerID(t *testing.T) {
	entries := []os.FileInfo{ownerTestEntry{uid: 1000, gid: 1001}, ownerTestEntry{uid: 0, gid: 0}, ownerTestEntry{uid: 5000, gid: 1001}, ownerTestEntry{uid: 1000, gid: 9000}}
	peer := newReusePeer(t, &reusePeer{exec: func(_ context.Context, channel ssh.Channel, command string) {
		if !strings.Contains(command, "getent passwd 0 1000 5000") || !strings.Contains(command, "getent group 0 1001 9000") {
			t.Error("lookup should batch unique numeric IDs", command)
		}
		io.WriteString(channel, "root:0\necs-user:1000\nmalformed\n__THUNDERSHIP_GROUPS__\nroot:0\ndevelopers:1001\n")
	}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := r.OwnerNames(ctx, entries)
	for key, expected := range map[string]string{"1000/1001": "ecs-user/developers", "0/0": "root/root", "5000/1001": "5000/developers", "1000/9000": "ecs-user/9000"} {
		if names[key] != expected {
			t.Fatal("owner name or numeric fallback", key, names[key], expected)
		}
	}
	if peer.connections.Load() != 1 || peer.commands.Load() != 1 {
		t.Fatal("owner lookup opened an extra transport or ran per-file queries")
	}
}

func TestRemoteOwnerLookupFailureDoesNotPreventListing(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{exitCode: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if names := r.OwnerNames(ctx, []os.FileInfo{ownerTestEntry{uid: 1000, gid: 1000}}); len(names) != 0 {
		t.Fatal("failed lookup should retain numeric display", names)
	}
}
