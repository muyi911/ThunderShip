package core

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func branchWorkflow() (Config, Workflow) {
	c := EmptyConfig()
	c.Servers = []Server{{ID: "a", Host: "a"}, {ID: "b", Host: "b"}}
	w := Workflow{ID: "w", Name: "branches", Steps: []Step{
		{ID: "a", Name: "A", Type: "server", Targets: &Targets{Servers: []string{"a"}, Concurrency: 1}},
		{ID: "upload-a", Name: "upload A", Type: "upload", ArtifactID: "asset", Timeout: 60, Files: []FileMapping{{ID: "ma", Source: "bin", Remote: "/home/work/app", Executable: true}}},
		{ID: "restart-a", Name: "supervisor", Type: "command", Command: "supervisorctl restart app", Timeout: 30},
		{ID: "b", Name: "B", Type: "server", Targets: &Targets{Servers: []string{"b"}, Concurrency: 1}},
		{ID: "upload-b", Name: "upload B", Type: "upload", ArtifactID: "asset", Timeout: 60, Files: []FileMapping{{ID: "mb", Source: "bin", Remote: "/home/work/xxx/app", Executable: true}}},
		{ID: "restart-b", Name: "shell", Type: "command", Command: "sh start.sh", Directory: "/home/work/xxx", Timeout: 30},
	}}
	w.Flow = &Flow{Nodes: []FlowNode{{ID: "a", Ref: "a"}, {ID: "ua", Ref: "upload-a"}, {ID: "ra", Ref: "restart-a"}, {ID: "b", Ref: "b"}, {ID: "ub", Ref: "upload-b"}, {ID: "rb", Ref: "restart-b"}}, Edges: []FlowEdge{{"a", "ua"}, {"ua", "ra"}, {"b", "ub"}, {"ub", "rb"}}}
	c.Artifacts = []ArtifactResource{{ID: "asset", Name: "service", Files: []FileMapping{{ID: "bin", Name: "program", Kind: "file"}}}}
	c.Workflows = []Workflow{w}
	return c, w
}

func TestExternalAssetValidationAndMissingInputs(t *testing.T) {
	c, w := branchWorkflow()
	if errs := ValidateWorkflow(w, c, false); len(errs) > 0 {
		t.Fatal(errs)
	}
	inputs, err := RunInputs(c, Node{WorkflowID: w.ID})
	if err != nil || len(inputs) != 1 || inputs[0].Error == "" {
		t.Fatal(inputs, err)
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "app")
	if err = os.WriteFile(file, []byte("version1"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Artifacts[0].Files[0].Local = file
	inputs, err = RunInputs(c, Node{WorkflowID: w.ID})
	if err != nil || len(inputs) != 1 || inputs[0].Error != "" {
		t.Fatal(inputs, err)
	}
	// An unused missing file in the same asset must not block this workflow.
	c.Artifacts[0].Files = append(c.Artifacts[0].Files, FileMapping{ID: "unused", Kind: "file", Local: filepath.Join(dir, "missing")})
	if errs := ValidateWorkflow(w, c, true); len(errs) > 0 {
		t.Fatal(errs)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	inputs, err = RunInputs(c, Node{WorkflowID: w.ID})
	if err != nil || len(inputs) != 1 || !strings.Contains(inputs[0].Error, "不存在") {
		t.Fatal(inputs, err)
	}
	if errs := ValidateWorkflow(w, c, true); len(errs) == 0 {
		t.Fatal("missing asset accepted at execution boundary")
	}
	if errs := ValidateWorkflow(w, c, false); len(errs) > 0 {
		t.Fatal("missing local file must not prevent editing configuration", errs)
	}
	fileMap := w.Steps[1].Files[0]
	fileMap.Source = "unknown"
	w.Steps[1].Files = []FileMapping{fileMap}
	if errs := ValidateWorkflow(w, c, false); len(errs) == 0 {
		t.Fatal("unknown asset input accepted")
	}
}

func TestAssetSnapshotSharedAcrossDestinationsAndWorkflows(t *testing.T) {
	c, w := branchWorkflow()
	file := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(file, []byte("one version"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Artifacts[0].Files[0].Local = file
	copy := w
	copy.ID = "second"
	c.Workflows = append(c.Workflows, copy)
	n := Node{Mode: "parallel", Children: []Node{{WorkflowID: w.ID}, {WorkflowID: copy.ID}}}
	inputs, err := RunInputs(c, n)
	if err != nil || len(inputs) != 1 {
		t.Fatal("shared asset was requested more than once", inputs, err)
	}
	frozen, err := Freeze(c, n, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := frozen.Workflows[0].Steps[1].Files[0]
	for _, wf := range frozen.Workflows {
		for _, st := range wf.Steps {
			if st.Type != "upload" {
				continue
			}
			if st.Files[0].Local != first.Local {
				t.Fatal("destinations did not share snapshot", st.Files)
			}
		}
	}
	if first.Remote != "/home/work/app" || frozen.Workflows[0].Steps[4].Files[0].Remote != "/home/work/xxx/app" {
		t.Fatal("destination paths mixed")
	}
	if err = os.WriteFile(file, []byte("new version"), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(first.Local)
	if err != nil || string(data) != "one version" {
		t.Fatal(string(data), err)
	}
	if c.Artifacts[0].Files[0].Local != file || c.Workflows[0].Steps[1].Files[0].Local != "" {
		t.Fatal("snapshot mutated saved configuration")
	}
}

func TestFolderAssetKeepsRelativePathsInOneSharedSnapshot(t *testing.T) {
	c, w := branchWorkflow()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "conf"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "conf", "app.yaml"), []byte("config"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Artifacts[0].Files[0].Local = dir
	c.Artifacts[0].Files[0].Kind = "folder"
	frozen, err := Freeze(c, Node{WorkflowID: w.ID}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a, b := frozen.Workflows[0].Steps[1], frozen.Workflows[0].Steps[4]
	if len(a.Files) != 1 || a.Files[0].Local != b.Files[0].Local || a.Files[0].Remote != "/home/work/app/conf/app.yaml" || b.Files[0].Remote != "/home/work/xxx/app/conf/app.yaml" || len(a.LockPaths) != 1 || a.LockPaths[0] != "/home/work/app" {
		t.Fatal(a, b)
	}
}

func TestServerContextsRequireExplicitTargetsAtJoin(t *testing.T) {
	c, w := branchWorkflow()
	w.Flow.Edges = append(w.Flow.Edges, FlowEdge{"ub", "ra"})
	if errs := ValidateWorkflow(w, c, false); !strings.Contains(strings.Join(errs, " "), "不同服务器") {
		t.Fatal(errs)
	}
	w.Flow.Edges = []FlowEdge{{"ua", "ra"}, {"b", "ub"}, {"ub", "rb"}}
	if errs := ValidateWorkflow(w, c, false); !strings.Contains(strings.Join(errs, " "), "前序服务器") {
		t.Fatal(errs)
	}
	c, w = branchWorkflow()
	w.Steps[4].Files[0].Remote = w.Steps[1].Files[0].Remote
	if errs := ValidateWorkflow(w, c, false); len(errs) > 0 {
		t.Fatal("different hosts cannot conflict on paths", errs)
	}
	c.Servers[1].Host = c.Servers[0].Host
	if errs := ValidateWorkflow(w, c, false); !strings.Contains(strings.Join(errs, " "), "上传路径重叠") {
		t.Fatal("host aliases bypassed path safety", errs)
	}
}

type deploymentSession struct {
	fakeSession
	run     func(Step) error
	publish func([]Artifact) error
}

func (s deploymentSession) Run(_ context.Context, st Step, _ func(string)) error { return s.run(st) }
func (s deploymentSession) Publish(_ context.Context, files []Artifact, _ Step, _ func(string, int64, int64, string)) error {
	return s.publish(files)
}

func TestBranchesUseOwnServerPathsAndPostUploadCommands(t *testing.T) {
	c, w := branchWorkflow()
	file := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(file, []byte("payload"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Artifacts[0].Files[0].Local = file
	var mu sync.Mutex
	uploaded := map[string]string{}
	commands := map[string]string{}
	e, err := NewEngine(t.TempDir(), func(_ context.Context, s Server) (Session, error) {
		return deploymentSession{run: func(st Step) error {
			mu.Lock()
			defer mu.Unlock()
			if uploaded[s.ID] == "" {
				t.Error("command ran before upload")
			}
			commands[s.ID] = st.Command
			if s.ID == "b" && st.Directory != "/home/work/xxx" {
				t.Error("wrong directory")
			}
			return nil
		}, publish: func(files []Artifact) error {
			mu.Lock()
			defer mu.Unlock()
			uploaded[s.ID] = files[0].Remote
			return nil
		}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = e.workflow(context.Background(), c, w, "run"); err != nil {
		t.Fatal(err)
	}
	if uploaded["a"] != "/home/work/app" || uploaded["b"] != "/home/work/xxx/app" || commands["a"] != "supervisorctl restart app" || commands["b"] != "sh start.sh" {
		t.Fatal(uploaded, commands)
	}
}

func TestTargetHostsProgressIndependentlyAndSerialFailureStopsLaterHosts(t *testing.T) {
	for _, serial := range []bool{false, true} {
		t.Run(map[bool]string{false: "parallel", true: "serial"}[serial], func(t *testing.T) {
			c := EmptyConfig()
			c.Servers = []Server{{ID: "a", Host: "a"}, {ID: "b", Host: "b"}}
			w := NormalizeWorkflow(workflow("w", "first"))
			w.Steps[0].Targets.Servers = []string{"a", "b"}
			w.Steps[0].Targets.Concurrency = 2
			if serial {
				w.Steps[0].Targets.Concurrency = 1
			}
			w.Steps = append(w.Steps, Step{ID: "second", Type: "command", Command: "second", Timeout: 3})
			w.Flow = nil
			release := make(chan struct{})
			advanced := make(chan struct{})
			started := make(chan string, 4)
			e, err := NewEngine(t.TempDir(), func(_ context.Context, s Server) (Session, error) {
				started <- s.ID
				return deploymentSession{run: func(st Step) error {
					if s.ID == "a" && st.Command == "first" {
						if serial {
							return errors.New("failed a")
						}
						<-release
					}
					if s.ID == "b" && st.Command == "second" {
						close(advanced)
					}
					return nil
				}}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- e.workflow(context.Background(), c, w, "run") }()
			if !serial {
				select {
				case <-advanced:
				case <-time.After(time.Second):
					close(release)
					t.Fatal("fast server waited for slow server")
				}
				close(release)
			}
			select {
			case err = <-done:
			case <-time.After(time.Second):
				t.Fatal("execution stalled")
			}
			if serial {
				if err == nil {
					t.Fatal("failure lost")
				}
				close(started)
				for sid := range started {
					if sid != "a" {
						t.Fatal("later server started after failure")
					}
				}
			} else if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestMultiHostReservationIsAtomicAndCancelable(t *testing.T) {
	g := NewGate()
	release, err := g.Acquire(context.Background(), "a", 1, 0, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err = g.acquire(ctx, []lease{{machine: "b", limit: 1}, {machine: "a", limit: 1}}, 0)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	other, err := g.Acquire(ctx2, "b", 1, 0, nil, nil)
	if err != nil {
		t.Fatal("partial reservation leaked", err)
	}
	other()
	release()
}

func TestAIDraftUsesOnlySelectedServersAndExternalAssets(t *testing.T) {
	for _, mode := range []string{"allowed", "server-denied", "asset-denied", "source-invalid"} {
		t.Run(mode, func(t *testing.T) {
			c, w := branchWorkflow()
			file := filepath.Join(t.TempDir(), "program")
			if err := os.WriteFile(file, []byte("binary"), 0600); err != nil {
				t.Fatal(err)
			}
			c.Artifacts[0].Files[0].Local = file
			if mode == "source-invalid" {
				w.Steps[1].Files[0].Source = "invented"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
				var payload struct {
					Messages []struct {
						Content string `json:"content"`
					} `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if strings.Contains(payload.Messages[1].Content, file) {
					t.Error("asset local path leaked into model context")
				}
				content, _ := json.Marshal(Draft{Workflow: &w})
				json.NewEncoder(out).Encode(map[string]any{"choices": []any{map[string]any{"finish_reason": "stop", "message": map[string]string{"content": string(content)}}}})
			}))
			defer srv.Close()
			req := AIRequest{Prompt: "deploy", ServerIDs: []string{"a", "b"}, ArtifactIDs: []string{"asset"}}
			if mode == "server-denied" {
				req.ServerIDs = []string{"a"}
			}
			if mode == "asset-denied" {
				c.Artifacts = append(c.Artifacts, ArtifactResource{ID: "other", Name: "other"})
				req.ArtifactIDs = []string{"other"}
			}
			d, err := Generate(context.Background(), c, Provider{BaseURL: srv.URL, Mode: "json", Model: "test"}, "", req)
			if mode == "server-denied" || mode == "asset-denied" {
				if err == nil {
					t.Fatal("unselected resource accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if mode == "source-invalid" {
				if len(d.Missing) == 0 {
					t.Fatal("unknown asset input accepted")
				}
				return
			}
			if len(d.Missing) != 0 || d.Workflow.Steps[1].ArtifactID != "asset" || d.Workflow.Steps[1].Files[0].Local != "" {
				t.Fatal(d)
			}
		})
	}
}
