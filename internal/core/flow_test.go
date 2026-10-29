package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestFlowValidationAndFailureIsolation(t *testing.T) {
	f := Flow{Nodes: []FlowNode{{ID: "a", Ref: "a"}, {ID: "b", Ref: "b"}, {ID: "join", Ref: "join"}, {ID: "ok", Ref: "ok"}}, Edges: []FlowEdge{{"a", "join"}, {"b", "join"}}}
	refs := map[string]bool{"a": true, "b": true, "join": true, "ok": true}
	if err := ValidateFlow(f, refs); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	ran, blocked := map[string]bool{}, map[string]bool{}
	err := RunFlow(context.Background(), f, func(n FlowNode) error {
		mu.Lock()
		ran[n.ID] = true
		mu.Unlock()
		if n.ID == "a" {
			return errors.New("fail")
		}
		return nil
	}, func(n FlowNode) { mu.Lock(); blocked[n.ID] = true; mu.Unlock() })
	if err == nil || ran["join"] || !blocked["join"] || !ran["b"] || !ran["ok"] {
		t.Fatal(ran, blocked, err)
	}
	f.Edges = append(f.Edges, FlowEdge{"join", "a"})
	if err := ValidateFlow(f, refs); err == nil {
		t.Fatal("cycle accepted")
	}
}
func TestFlowFanInWaitsForAllParents(t *testing.T) {
	f := Flow{Nodes: []FlowNode{{ID: "a", Ref: "a"}, {ID: "b", Ref: "b"}, {ID: "c", Ref: "c"}}, Edges: []FlowEdge{{"a", "c"}, {"b", "c"}}}
	var mu sync.Mutex
	parents := map[string]bool{}
	err := RunFlow(context.Background(), f, func(n FlowNode) error {
		mu.Lock()
		defer mu.Unlock()
		if n.ID == "c" {
			if !parents["a"] || !parents["b"] {
				t.Error("fan-in started early")
			}
		} else {
			parents[n.ID] = true
		}
		return nil
	}, func(FlowNode) { t.Error("unexpected block") })
	if err != nil {
		t.Fatal(err)
	}
}
func TestCompositionReferencesAndCycles(t *testing.T) {
	c := EmptyConfig()
	c.Workflows = []Workflow{workflow("leaf", "true")}
	c.Servers = []Server{{ID: "s"}}
	c.Combinations = []Combination{{ID: "inner", Root: Node{Graph: &Flow{Nodes: []FlowNode{{ID: "n", Ref: "leaf"}}}}}, {ID: "outer", Root: Node{WorkflowID: "inner"}}}
	selected, err := Selected(c, Node{WorkflowID: "outer"})
	if err != nil || len(selected) != 1 || selected[0].ID != "leaf" {
		t.Fatal(selected, err)
	}
	c.Combinations[0].Root = Node{WorkflowID: "outer"}
	if err := ValidateNode(Node{WorkflowID: "outer"}, c); err == nil {
		t.Fatal("reference cycle accepted")
	}
}
func TestFlowRequiresOrderingForOverlappingUploads(t *testing.T) {
	c := EmptyConfig()
	c.Servers = []Server{{ID: "s"}}
	w := workflow("upload", "")
	w.Steps = []Step{
		{ID: "a", Type: "upload", Timeout: 60, Files: []FileMapping{{Local: "a", Remote: "/srv/app"}}},
		{ID: "b", Type: "upload", Timeout: 60, Files: []FileMapping{{Local: "b", Remote: "/srv/app/config"}}},
	}
	w.Flow = &Flow{Nodes: []FlowNode{{ID: "first", Ref: "a"}, {ID: "second", Ref: "b"}}}
	if errs := ValidateWorkflow(w, c, false); !strings.Contains(strings.Join(errs, " "), "上传路径重叠") {
		t.Fatal("unordered uploads accepted", errs)
	}
	w.Flow.Edges = []FlowEdge{{"first", "second"}}
	if errs := ValidateWorkflow(w, c, false); len(errs) != 0 {
		t.Fatal(errs)
	}
}
func TestAtomicReplacementInAppData(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows AppData redirection regression")
	}
	root, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "shell-workflow-storage-check-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	target := filepath.Join(dir, "配置.json")
	for i := 0; i < 3; i++ {
		if err := WriteJSON(target, map[string]int{"version": i}); err != nil {
			t.Fatal(err)
		}
	}
}

type uploadModeSession struct {
	fakeSession
	modes *[]bool
}

func (s uploadModeSession) OpenTransfer(_ context.Context, server Server) (Session, error) {
	*s.modes = append(*s.modes, server.SudoUpload)
	return s.fakeSession, nil
}

func TestUploadSudoIsIndependentOfCommandSudo(t *testing.T) {
	var mu sync.Mutex
	commands := []string{}
	var actual []bool
	var uploadModes []bool
	e, err := NewEngine(t.TempDir(), func(_ context.Context, s Server) (Session, error) {
		actual = append(actual, s.SudoUpload)
		return uploadModeSession{fakeSession{&mu, &commands}, &uploadModes}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "binary")
	if err := os.WriteFile(file, []byte("binary"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, step := range []Step{
		{Type: "command", Command: "true", Sudo: true, Timeout: 3},
		{Type: "upload", Files: []FileMapping{{Local: file, Remote: "/srv/binary"}}, Sudo: true, Timeout: 3},
		{Type: "upload", Files: []FileMapping{{Local: file, Remote: "/srv/binary"}}, Timeout: 3},
	} {
		if err := e.step(context.Background(), Server{}, step, func(string, string, string, int64, int64) {}); err != nil {
			t.Fatal(err)
		}
	}
	if len(actual) != 3 || actual[0] || actual[1] || actual[2] || len(uploadModes) != 2 || !uploadModes[0] || uploadModes[1] {
		t.Fatal("sudo leaked between operations", actual)
	}
}
