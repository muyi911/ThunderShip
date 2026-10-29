package core

import (
	"context"
	"encoding/base64"
	"reflect"
	"testing"
	"time"
)

func TestTerminalWaitPollWakesForOutputAndDrainsFinalBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	terminal := &Terminal{ctx: ctx, output: make(chan []byte, 128), done: make(chan struct{})}
	read := make(chan TerminalRead, 1)
	go func() { read <- terminal.WaitPoll() }()
	terminal.output <- []byte("\b \b")
	select {
	case result := <-read:
		if result.Data != base64.StdEncoding.EncodeToString([]byte("\b \b")) || result.Closed {
			t.Fatal(result)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("output waited for a polling interval")
	}
	terminal.output <- []byte("final ")
	terminal.output <- []byte("output")
	close(terminal.done)
	result := terminal.WaitPoll()
	if !result.Closed || result.Data != base64.StdEncoding.EncodeToString([]byte("final output")) {
		t.Fatal("lost final output", result)
	}
	terminal.done = make(chan struct{})
	cancel()
	go func() { read <- terminal.WaitPoll() }()
	select {
	case <-read:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("cancelled read remained blocked")
	}
}

func TestRemotePathCompletionUsesExistingSFTPWithoutExecutingInput(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	r, err := Dial(context.Background(), peer.server, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for _, dir := range []string{"/usr", "/user data", "/home/test/assets"} {
		if err := r.SFTP.MkdirAll(dir); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"/users.txt", "/home/test/.hidden", "/home/test/a b.txt"} {
		file, err := r.SFTP.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		file.Close()
	}
	for _, test := range []struct {
		word     string
		onlyDirs bool
		want     []string
	}{
		{"/u", true, []string{"/user data/", "/usr/"}},
		{"/u", false, []string{"/user data/", "/users.txt", "/usr/"}},
		{"a", false, []string{"a b.txt", "assets/"}},
		{"./a", true, []string{"./assets/"}},
		{".", false, []string{".hidden"}},
		{"assets/", false, []string{}},
		{"$(touch marker)", false, []string{}},
	} {
		got, err := r.CompletePath("/home/test", test.word, test.onlyDirs)
		if err != nil || !reflect.DeepEqual(got, test.want) {
			t.Fatalf("%q: %v %v, wanted %v", test.word, got, err, test.want)
		}
	}
	if peer.commands.Load() != 0 || peer.connections.Load() != 1 {
		t.Fatal("completion executed a command or reconnected")
	}
}
