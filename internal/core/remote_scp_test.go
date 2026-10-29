package core

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func TestSCPPublishRetainsAtomicReplacementAndFailureChecks(t *testing.T) {
	for _, scenario := range []string{"success", "sudo", "empty", "rejected", "exit failure", "size mismatch", "missing scp", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			peer := newReusePeer(t, &reusePeer{limit: 1})
			if scenario == "exit failure" {
				peer.exitCode = 1
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var uploads atomic.Int32
			peer.exec = func(_ context.Context, channel ssh.Channel, command string) {
				uploads.Add(1)
				prefix := "scp -t -- "
				if scenario == "sudo" {
					prefix = "sudo -n -u 'root' -- " + prefix
				}
				if !strings.HasPrefix(command, prefix) {
					t.Errorf("incorrect SCP identity or command: %s", command)
					return
				}
				quoted := strings.TrimPrefix(command, prefix)
				target := strings.ReplaceAll(strings.TrimSuffix(strings.TrimPrefix(quoted, "'"), "'"), "'\"'\"'", "'")
				if scenario == "missing scp" {
					io.WriteString(channel.Stderr(), "scp: command not found")
					return
				}
				channel.Write([]byte{0})
				reader := bufio.NewReader(channel)
				header, err := reader.ReadString('\n')
				var size int64
				if _, scanErr := fmt.Sscanf(header, "C0600 %d upload\n", &size); err != nil || scanErr != nil {
					t.Errorf("invalid SCP header: %q %v %v", header, err, scanErr)
					return
				}
				channel.Write([]byte{0})
				if scenario == "canceled" {
					cancel()
					return
				}
				payload := make([]byte, size)
				if _, err := io.ReadFull(reader, payload); err != nil {
					return
				}
				if marker, err := reader.ReadByte(); err != nil || marker != 0 {
					t.Errorf("missing SCP completion marker: %d %v", marker, err)
					return
				}
				if scenario == "size mismatch" {
					payload = payload[:len(payload)-1]
				}
				file, err := peer.files.FilePut.Filewrite(&sftp.Request{Method: "Put", Filepath: target, Flags: 2 | 16})
				if err != nil {
					t.Error(err)
					return
				}
				if _, err := file.WriteAt(payload, 0); err != nil {
					t.Error(err)
				}
				if closer, ok := file.(io.Closer); ok {
					closer.Close()
				}
				if scenario == "rejected" {
					channel.Write([]byte("\x01disk full\n"))
					return
				}
				channel.Write([]byte{0})
				io.Copy(io.Discard, reader)
			}
			r, err := Dial(ctx, peer.server, "test", false)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			r.server.SudoUpload = scenario == "sudo"
			target := "/app ' with spaces"
			original, err := r.SFTP.Create(target)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := original.Write([]byte("original")); err != nil {
				t.Fatal(err)
			}
			original.Close()
			payload := bytes.Repeat([]byte("new executable\n"), 10000)
			if scenario == "empty" {
				payload = nil
			}
			source := filepath.Join(t.TempDir(), "app")
			if err := os.WriteFile(source, payload, 0600); err != nil {
				t.Fatal(err)
			}
			var published bool
			err = r.Publish(ctx, []Artifact{{Local: source, Remote: target, Size: int64(len(payload)), Executable: true}}, Step{UploadProtocol: "scp"}, func(_ string, _, _ int64, state string) { published = published || state == "published" })
			success := scenario == "success" || scenario == "sudo" || scenario == "empty"
			if (err == nil) != success || published != success || uploads.Load() != 1 {
				t.Fatal("incorrect SCP publication", err, published, uploads.Load())
			}
			if scenario == "missing scp" && !strings.Contains(err.Error(), "command not found") {
				t.Fatal("missing server dependency was hidden", err)
			}
			file, err := peer.files.FileGet.Fileread(&sftp.Request{Method: "Get", Filepath: target, Flags: 1})
			if err != nil {
				t.Fatal(err)
			}
			want := []byte("original")
			if success {
				want = payload
			}
			got := make([]byte, len(want))
			if _, err := file.ReadAt(got, 0); err != nil && err != io.EOF || !bytes.Equal(got, want) {
				t.Fatal("SCP corrupted original or published incorrect content", err)
			}
			if peer.connections.Load() != 1 {
				t.Fatal("SCP opened an extra SSH connection")
			}
		})
	}
}

func TestSCPDownloadFailureDoesNotReplaceLocalFile(t *testing.T) {
	for _, scenario := range []string{"success", "short transfer", "rejected", "wrong size", "exit failure"} {
		t.Run(scenario, func(t *testing.T) {
			peer := newReusePeer(t, &reusePeer{limit: 1})
			if scenario == "exit failure" {
				peer.exitCode = 1
			}
			payload := bytes.Repeat([]byte("download\n"), 10000)
			peer.exec = func(_ context.Context, channel ssh.Channel, command string) {
				if command != "scp -f -- '/app'" {
					t.Error("unexpected download command", command)
					return
				}
				var ack [1]byte
				io.ReadFull(channel, ack[:])
				size := len(payload)
				if scenario == "wrong size" {
					size++
				}
				fmt.Fprintf(channel, "C0644 %d app\n", size)
				if _, err := io.ReadFull(channel, ack[:]); err != nil {
					return
				}
				if scenario == "short transfer" {
					channel.Write(payload[:100])
					return
				}
				channel.Write(payload)
				if scenario == "rejected" {
					channel.Write([]byte("\x01read failed\n"))
					return
				}
				channel.Write([]byte{0})
				io.ReadFull(channel, ack[:])
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r, err := Dial(ctx, peer.server, "test", false)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			remote, err := r.SFTP.Create("/app")
			if err != nil {
				t.Fatal(err)
			}
			remote.Write(payload)
			remote.Close()
			target := filepath.Join(t.TempDir(), "download.bin")
			os.WriteFile(target, []byte("original"), 0600)
			err = WriteAtomic(target, func(out *os.File) error { return r.Download(ctx, "/app", out, "scp") })
			want := []byte("original")
			if scenario == "success" {
				want = payload
			}
			actual, readErr := os.ReadFile(target)
			if (err == nil) != (scenario == "success") || readErr != nil || !bytes.Equal(actual, want) {
				t.Fatal("download failed to preserve destination", scenario, err, readErr)
			}
		})
	}
}

type protocolSession struct {
	fakeSession
	protocols chan string
}

func (s protocolSession) Publish(_ context.Context, _ []Artifact, options Step, _ func(string, int64, int64, string)) error {
	s.protocols <- options.UploadProtocol
	return nil
}

func TestWorkflowUsesConfiguredUploadProtocol(t *testing.T) {
	for _, protocol := range []string{"sftp", "scp"} {
		t.Run(protocol, func(t *testing.T) {
			protocols := make(chan string, 2)
			e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) {
				return protocolSession{protocols: protocols}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(t.TempDir(), "app")
			if err := os.WriteFile(file, []byte("payload"), 0600); err != nil {
				t.Fatal(err)
			}
			c := EmptyConfig()
			c.UploadProtocol = protocol
			c.Servers = []Server{{ID: "s", Host: "test"}}
			w := NormalizeWorkflow(workflow("w", "unused"))
			w.Steps[1] = Step{ID: "upload", Type: "upload", Timeout: 3, Files: []FileMapping{{Local: file, Remote: "/app"}}}
			w.Flow = nil
			if err := e.workflow(context.Background(), c, w, "run"); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-protocols:
				if got != protocol {
					t.Fatal("workflow ignored upload preference", got)
				}
			default:
				t.Fatal("workflow did not upload")
			}
		})
	}
}
