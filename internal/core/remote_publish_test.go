package core

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
)

type uploadReadGuard struct{ reads atomic.Int32 }

func (g *uploadReadGuard) Fileread(*sftp.Request) (io.ReaderAt, error) {
	g.reads.Add(1)
	return nil, fmt.Errorf("upload must not download the file again")
}

type uploadFaultWriter struct {
	sftp.FileWriter
	drop, closeError, writeError bool
}

func (w uploadFaultWriter) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	file, err := w.FileWriter.Filewrite(req)
	if err != nil {
		return nil, err
	}
	return uploadFaultHandle{file, w.drop, w.closeError, w.writeError}, nil
}

type uploadFaultHandle struct {
	io.WriterAt
	drop, closeError, writeError bool
}

func (f uploadFaultHandle) WriteAt(p []byte, offset int64) (int, error) {
	if f.writeError && offset >= 32768 {
		return 0, fmt.Errorf("server write failed")
	}
	if f.drop {
		return len(p), nil
	}
	return f.WriterAt.WriteAt(p, offset)
}

func (f uploadFaultHandle) Close() error {
	if closer, ok := f.WriterAt.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	if f.closeError {
		return fmt.Errorf("server could not close uploaded file")
	}
	return nil
}

func TestPublishWithoutReadbackRetainsWriteSizeAndCloseChecks(t *testing.T) {
	for _, scenario := range []string{"success", "remote size mismatch", "local size changed", "close failure", "write failure"} {
		t.Run(scenario, func(t *testing.T) {
			peer := newReusePeer(t, &reusePeer{})
			reader := peer.files.FileGet
			target, err := peer.files.FilePut.Filewrite(&sftp.Request{Method: "Put", Filepath: "/app", Flags: 2 | 8 | 16})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := target.WriteAt([]byte("original"), 0); err != nil {
				t.Fatal(err)
			}
			if closer, ok := target.(io.Closer); ok {
				closer.Close()
			}
			guard := &uploadReadGuard{}
			peer.files.FileGet = guard
			peer.files.FilePut = uploadFaultWriter{peer.files.FilePut, scenario == "remote size mismatch", scenario == "close failure", scenario == "write failure"}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			r, err := Dial(ctx, peer.server, "test", false)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			payload := bytes.Repeat([]byte("new binary\n"), 100000)
			source := filepath.Join(t.TempDir(), "app")
			if err := os.WriteFile(source, payload, 0600); err != nil {
				t.Fatal(err)
			}
			file := Artifact{Local: source, Remote: "/app", Size: int64(len(payload))}
			if scenario == "local size changed" {
				file.Size++
			}
			var published, prepared bool
			err = r.Publish(ctx, []Artifact{file}, Step{}, func(_ string, _, _ int64, state string) {
				prepared = prepared || state == "prepared"
				published = published || state == "published"
				if state == "verified" {
					t.Error("upload incorrectly reported a checksum match")
				}
			})
			success := scenario == "success"
			if (err == nil) != success || published != success || prepared != success {
				t.Fatal("incorrect publication result", err, prepared, published)
			}
			if guard.reads.Load() != 0 {
				t.Fatal("upload downloaded content for verification")
			}
			actual, err := reader.Fileread(&sftp.Request{Method: "Get", Filepath: "/app", Flags: 1})
			if err != nil {
				t.Fatal(err)
			}
			want := []byte("original")
			if success {
				want = payload
			}
			got := make([]byte, len(want))
			_, err = actual.ReadAt(got, 0)
			if err != nil && err != io.EOF || !bytes.Equal(got, want) {
				t.Fatal("published content changed or failed preparation overwrote original", err)
			}
			entries, err := r.SFTP.ReadDir("/")
			if err != nil || len(entries) != 1 || entries[0].Name() != "app" {
				t.Fatal("temporary files were not cleaned up", entries, err)
			}
		})
	}
}

type uploadPipelineWriter struct {
	sftp.FileWriter
	delay   time.Duration
	overlap chan struct{}
	active  atomic.Int32
	once    sync.Once
}

func (w *uploadPipelineWriter) Filewrite(req *sftp.Request) (io.WriterAt, error) {
	file, err := w.FileWriter.Filewrite(req)
	if err != nil {
		return nil, err
	}
	return uploadPipelineHandle{uploadFaultHandle{WriterAt: file}, w}, nil
}

type uploadPipelineHandle struct {
	uploadFaultHandle
	owner *uploadPipelineWriter
}

func (f uploadPipelineHandle) WriteAt(p []byte, offset int64) (int, error) {
	w := f.owner
	active := w.active.Add(1)
	defer w.active.Add(-1)
	if w.overlap != nil {
		if active > 1 {
			w.once.Do(func() { close(w.overlap) })
		}
		select {
		case <-w.overlap:
		case <-time.After(2 * time.Second):
			return 0, fmt.Errorf("upload waited for each packet instead of pipelining writes")
		}
	}
	if w.delay > 0 {
		time.Sleep(w.delay)
	}
	return f.uploadFaultHandle.WriteAt(p, offset)
}

func TestPublishPipelinesWritesOnOneChannel(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{limit: 1})
	writer := &uploadPipelineWriter{FileWriter: peer.files.FilePut, overlap: make(chan struct{})}
	peer.files.FilePut = writer
	payload := bytes.Repeat([]byte("pipeline\n"), 250000)
	source := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := Dial(ctx, peer.server, "test", false)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.Publish(ctx, []Artifact{{Local: source, Remote: "/app", Size: int64(len(payload))}}, Step{}, func(string, int64, int64, string) {}); err != nil {
		t.Fatal(err)
	}
	if peer.connections.Load() != 1 || peer.transfers.Load() != 1 {
		t.Fatal("pipelining opened extra connections or channels", peer.connections.Load(), peer.transfers.Load())
	}
	file, err := r.SFTP.Open("/app")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatal("concurrent upload changed the published content", err)
	}
}

func TestCanceledPublishPipelinePreservesOriginalAndTransport(t *testing.T) {
	peer := newReusePeer(t, &reusePeer{})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, err := DialSSH(ctx, peer.server, "test")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	transferCtx, stop := context.WithCancel(ctx)
	defer stop()
	transfer, err := r.OpenTransfer(transferCtx, peer.server)
	if err != nil {
		t.Fatal(err)
	}
	defer transfer.Close()
	file, err := transfer.(*Remote).SFTP.Create("/app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(source, bytes.Repeat([]byte("x"), 2*1024*1024), 0600); err != nil {
		t.Fatal(err)
	}
	published := false
	err = transfer.Publish(transferCtx, []Artifact{{Local: source, Remote: "/app", Size: 2 * 1024 * 1024}}, Step{}, func(_ string, n, _ int64, state string) {
		if state == "uploading" && n > 0 {
			stop()
		}
		published = published || state == "published"
	})
	if err == nil || published {
		t.Fatal("canceled transfer published a partial file", err)
	}
	check, err := r.OpenTransfer(ctx, peer.server)
	if err != nil {
		t.Fatal("canceling one transfer closed the shared transport", err)
	}
	defer check.Close()
	file, err = check.(*Remote).SFTP.Open("/app")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil || string(got) != "original" || peer.connections.Load() != 1 {
		t.Fatal("cancellation changed the original or opened another connection", err, peer.connections.Load())
	}
}

func BenchmarkPublish30MiB(b *testing.B) {
	peer := newReusePeer(b, &reusePeer{limit: 1})
	// Model a small per-packet acknowledgement delay, without using an external server.
	peer.files.FilePut = &uploadPipelineWriter{FileWriter: peer.files.FilePut, delay: 4 * time.Millisecond}
	payload := bytes.Repeat([]byte("x"), 30*1024*1024)
	source := filepath.Join(b.TempDir(), "app")
	if err := os.WriteFile(source, payload, 0600); err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	r, err := Dial(ctx, peer.server, "test", false)
	if err != nil {
		b.Fatal(err)
	}
	defer r.Close()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := r.Publish(ctx, []Artifact{{Local: source, Remote: "/app", Size: int64(len(payload))}}, Step{}, func(string, int64, int64, string) {}); err != nil {
			b.Fatal(err)
		}
	}
}
