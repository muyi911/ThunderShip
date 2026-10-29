package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFileExpansionAndSnapshot(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "文件夹 with spaces")
	if e := os.MkdirAll(filepath.Join(src, "sub"), 0700); e != nil {
		t.Fatal(e)
	}
	file := filepath.Join(src, "sub", "服务.bin")
	if e := os.WriteFile(file, []byte("old"), 0600); e != nil {
		t.Fatal(e)
	}
	m := FileMapping{ID: "m", Local: src, Remote: "/opt/a"}
	files, e := Expand([]FileMapping{m})
	if e != nil || len(files) != 1 || files[0].Remote != "/opt/a/sub/服务.bin" {
		t.Fatalf("%+v %v", files, e)
	}
	if _, e = Expand([]FileMapping{m, {Local: file, Remote: "/opt/a/sub/服务.bin"}}); e == nil {
		t.Fatal("duplicate remote accepted")
	}
	w := Workflow{ID: "w", Steps: []Step{{Type: "upload", Files: []FileMapping{m}}}}
	c := EmptyConfig()
	c.Workflows = []Workflow{w}
	frozen, e := Freeze(c, Node{WorkflowID: "w"}, filepath.Join(dir, "run"))
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(frozen.Workflows[0].Steps[0].LockPaths, []string{"/opt/a"}) {
		t.Fatal("folder lock scope was lost during snapshot")
	}
	os.WriteFile(file, []byte("new"), 0600)
	b, e := os.ReadFile(frozen.Workflows[0].Steps[0].Files[0].Local)
	if e != nil || string(b) != "old" {
		t.Fatalf("snapshot changed: %s %v", b, e)
	}
	for _, p := range []string{"/", "../x", "/a/../b", "/a\\b", "/a\nfile"} {
		if ValidRemote(p) {
			t.Errorf("accepted %q", p)
		}
	}
}
func TestAtomicConfigReplacement(t *testing.T) {
	f := filepath.Join(t.TempDir(), "config.json")
	for i := 0; i < 3; i++ {
		if e := WriteJSON(f, map[string]int{"version": i}); e != nil {
			t.Fatal(e)
		}
	}
	b, e := os.ReadFile(f)
	if e != nil || !strings.Contains(string(b), "2") {
		t.Fatal(string(b), e)
	}
}
func TestGateConflictsAndCrossHostParallel(t *testing.T) {
	g := NewGate()
	release, e := g.Acquire(context.Background(), "host", 2, 4, []string{"/opt/a"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if r, e := g.Acquire(ctx, "host", 2, 4, []string{"/opt/a/config"}, nil); e == nil {
		r()
		t.Fatal("overlapping paths ran concurrently")
	}
	r, e := g.Acquire(context.Background(), "host", 2, 4, []string{"/opt/b"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	r()
	r, e = g.Acquire(context.Background(), "other-host", 1, 4, []string{"/opt/a"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	r()
}

func TestUnlimitedHostConcurrencyKeepsSafetyGates(t *testing.T) {
	g := NewGate()
	acquire := func(limit int, paths, resources []string) func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		release, err := g.Acquire(ctx, "host", limit, 3, paths, resources)
		if err != nil {
			t.Fatal(err)
		}
		return release
	}
	blocked := func(machine string, limit int, paths, resources []string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if release, err := g.Acquire(ctx, machine, limit, 3, paths, resources); !errors.Is(err, context.DeadlineExceeded) {
			if release != nil {
				release()
			}
			t.Fatalf("expected safety gate to block, got %v", err)
		}
	}
	release := acquire(0, []string{"/opt/a"}, []string{"systemd:a"})
	defer release()
	release = acquire(0, []string{"/opt/b"}, nil)
	defer release()
	blocked("host", 0, []string{"/opt/a/config"}, nil)
	blocked("host", 0, nil, []string{"systemd:a"})
	blocked("host", 1, nil, nil)
	release = acquire(0, []string{"/opt/c"}, nil)
	defer release()
	blocked("other-host", 0, nil, nil)
}

func TestHostLimitUsesStrictestExplicitAliasLimit(t *testing.T) {
	s := Server{Host: "example", Port: 22}
	for _, test := range []struct {
		name    string
		current int
		aliases []Server
		want    int
	}{
		{"unlimited", 0, []Server{{Host: "example", Port: 22}}, 0},
		{"explicit", 4, []Server{{Host: "example", Port: 22}}, 4},
		{"limited alias", 0, []Server{{Host: "example", Port: 22, Concurrency: 2}}, 2},
		{"strictest alias", 4, []Server{{Host: "example", Port: 22, Concurrency: 2}}, 2},
		{"other host", 0, []Server{{Host: "other", Port: 22, Concurrency: 1}}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			s.Concurrency = test.current
			if got := HostLimit(Config{Servers: test.aliases}, s); got != test.want {
				t.Fatalf("HostLimit = %d, want %d", got, test.want)
			}
		})
	}
	s.Machine = "physical-host"
	if got := HostLimit(Config{Servers: []Server{{Host: "other", Machine: "PHYSICAL-HOST", Concurrency: 1}}}, s); got != 1 {
		t.Fatalf("physical host alias limit was ignored: %d", got)
	}
}

type fakeSession struct {
	mu       *sync.Mutex
	commands *[]string
}

func (f fakeSession) Close()                      {}
func (f fakeSession) CheckFiles([]Artifact) error { return nil }
func (f fakeSession) Publish(context.Context, []Artifact, Step, func(string, int64, int64, string)) error {
	return nil
}
func (f fakeSession) Run(_ context.Context, s Step, _ func(string)) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	*f.commands = append(*f.commands, s.Command)
	if s.Command == "fail" {
		return errors.New("expected failure")
	}
	return nil
}
func workflow(id, command string) Workflow {
	return Workflow{ID: id, Name: id, Servers: []string{"s"}, Concurrency: 1, Steps: []Step{{ID: "step", Name: command, Type: "command", Command: command, Timeout: 3}}}
}
func TestNestedFailureIsolation(t *testing.T) {
	var mu sync.Mutex
	commands := []string{}
	e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) { return fakeSession{&mu, &commands}, nil })
	if err != nil {
		t.Fatal(err)
	}
	c := EmptyConfig()
	c.Servers = []Server{{ID: "s", Host: "test", Port: 22, Concurrency: 2}}
	c.Workflows = []Workflow{workflow("a", "fail"), workflow("b", "independent"), workflow("c", "must-not-run")}
	n := Node{Mode: "parallel", Children: []Node{{Mode: "serial", Children: []Node{{WorkflowID: "a"}, {WorkflowID: "c"}}}, {WorkflowID: "b"}}}
	done := make(chan Event, 1)
	e.OnEvent = func(v Event) {
		if v.Workflow == "" && v.Status == "failed" {
			done <- v
		}
	}
	id, err := e.Start(context.Background(), c, n, "test")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("scheduler stalled")
	}
	mu.Lock()
	joined := strings.Join(commands, ",")
	mu.Unlock()
	if !strings.Contains(joined, "independent") || strings.Contains(joined, "must-not-run") {
		t.Fatal(joined)
	}
	blocked := false
	for _, ev := range e.Events(id, 0) {
		if ev.Workflow == "c" && ev.Status == "blocked" {
			blocked = true
		}
	}
	if !blocked {
		t.Fatal("serial successor not blocked")
	}
}
func TestCancellationWaitsForStartedHosts(t *testing.T) {
	started := make(chan struct{}, 2)
	cleanup := make(chan struct{})
	e, err := NewEngine(t.TempDir(), func(ctx context.Context, _ Server) (Session, error) {
		started <- struct{}{}
		<-ctx.Done()
		<-cleanup
		return nil, &UnknownError{ctx.Err()}
	})
	if err != nil {
		t.Fatal(err)
	}
	c := EmptyConfig()
	c.Servers = []Server{{ID: "a", Host: "a", Concurrency: 1}, {ID: "b", Host: "b", Concurrency: 1}, {ID: "c", Host: "c", Concurrency: 1}}
	w := workflow("test", "unused")
	w.Servers = []string{"a", "b", "c"}
	w.Concurrency = 2
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- e.workflow(ctx, c, w, "test") }()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("hosts did not start")
		}
	}
	cancel()
	select {
	case <-done:
		t.Fatal("workflow returned before started hosts finished")
	case <-time.After(30 * time.Millisecond):
	}
	close(cleanup)
	select {
	case err = <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		var unknown *UnknownError
		if !errors.As(err, &unknown) {
			t.Fatal("unknown host result was lost on cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation stalled")
	}
}
func TestModelListAndPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/v1/models" || r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("wrong endpoint/auth", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"z"},{"id":"a"},{"id":"z"}]}`)
	}))
	defer srv.Close()
	models, e := ListModels(context.Background(), Provider{BaseURL: srv.URL + "/custom/v1/"}, "test-key")
	if e != nil || strings.Join(models, ",") != "a,z" {
		t.Fatal(models, e)
	}
}
func TestCompletionCompatibilityAndAuthFailure(t *testing.T) {
	for _, status := range []int{400, 401} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := calls.Add(1)
				if n == 1 {
					w.WriteHeader(status)
					fmt.Fprint(w, `{"error":{"message":"unsupported response_format test-key"}}`)
					return
				}
				var p map[string]any
				json.NewDecoder(r.Body).Decode(&p)
				if _, ok := p["response_format"]; ok {
					t.Error("fallback retained format")
				}
				fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`)
			}))
			defer srv.Close()
			_, e := Completion(context.Background(), Provider{BaseURL: srv.URL, Model: "test", Mode: "auto"}, "test-key", "json", "json", map[string]any{})
			if status == 400 && (e != nil || calls.Load() != 2) {
				t.Fatal(calls.Load(), e)
			}
			if status == 401 && (e == nil || calls.Load() != 1 || strings.Contains(e.Error(), "test-key")) {
				t.Fatal(calls.Load(), e)
			}
		})
	}
}
func TestSchemaAndAIReferenceValidation(t *testing.T) {
	schema := Schema(reflect.TypeOf(Draft{}))
	if _, e := json.Marshal(schema); e != nil {
		t.Fatal("schema is cyclic", e)
	}
	if schema["type"] != "object" {
		t.Fatal("root must be object")
	}
	w := workflow("current", "secret-command")
	w.Steps[0].CommandMode = "lines"
	w.Steps[0].Commands = []string{"secret-line"}
	w.Steps[0].Name = "run"
	w.Servers = []string{"s"}
	c := EmptyConfig()
	c.Servers = []Server{{ID: "s", Name: "server"}}
	c.Workflows = []Workflow{w}
	for _, bad := range []bool{false, true} {
		t.Run(fmt.Sprint(bad), func(t *testing.T) {
			candidate := w
			if bad {
				candidate.Servers = []string{"not-selected"}
			}
			d := Draft{Summary: "test", Missing: []string{}, Workflow: &candidate, Combination: &Combination{Name: "combined", Root: Node{Mode: "serial", Children: []Node{{WorkflowID: "current"}}}}}
			b, _ := json.Marshal(d)
			srv := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				var payload struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				json.NewDecoder(r.Body).Decode(&payload)
				if strings.Contains(payload.Messages[1].Content, "secret-command") || strings.Contains(payload.Messages[1].Content, "secret-line") {
					t.Error("scripts leaked without opt-in")
				}
				json.NewEncoder(out).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(b)}}}})
			}))
			defer srv.Close()
			result, e := Generate(context.Background(), c, Provider{BaseURL: srv.URL, Mode: "json", Model: "test"}, "", AIRequest{Prompt: "compose", Workflow: &w, ServerIDs: []string{"s"}})
			if bad && e == nil {
				t.Fatal("unselected server accepted")
			}
			if !bad {
				if e != nil {
					t.Fatal(e)
				}
				if result.Combination.Root.WorkflowID != "" || result.Combination.Root.Children[0].WorkflowID != "current" {
					t.Fatal("group corrupted")
				}
			}
		})
	}
}
