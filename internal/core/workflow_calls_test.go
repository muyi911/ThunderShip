package core

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func calledWorkflowConfig(t *testing.T) (Config, Workflow) {
	t.Helper()
	c := EmptyConfig()
	c.Servers = []Server{{ID: "a", Host: "a", Concurrency: 1}}
	file := filepath.Join(t.TempDir(), "app")
	if err := os.WriteFile(file, []byte("one-version"), 0600); err != nil {
		t.Fatal(err)
	}
	c.Artifacts = []ArtifactResource{{ID: "asset", Name: "app", Files: []FileMapping{{ID: "bin", Kind: "file", Local: file}}}}
	child := Workflow{ID: "child", Name: "child", Steps: []Step{
		{ID: "target", Type: "server", Targets: &Targets{Servers: []string{"a"}, Concurrency: 1, Resources: []string{"app"}}},
		{ID: "upload", Type: "upload", ArtifactID: "asset", Files: []FileMapping{{ID: "m", Source: "bin", Remote: "/opt/app/app"}}, Timeout: 10},
		{ID: "command", Type: "command", Command: "child", Timeout: 10},
	}}
	parent := Workflow{ID: "parent", Name: "parent", Steps: []Step{
		{ID: "target", Type: "server", Targets: &Targets{Servers: []string{"a"}, Concurrency: 1, Resources: []string{"app"}}},
		{ID: "before", Type: "command", Command: "before", Timeout: 10},
		{ID: "call", Name: "call child", Type: "workflow", WorkflowID: "child"},
		{ID: "after", Type: "command", Command: "after", Timeout: 10},
	}}
	f := SequentialFlow(parent.Steps)
	f.Nodes = append([]FlowNode{{ID: "start", Ref: StartRef}}, f.Nodes...)
	f.Edges = append([]FlowEdge{{Source: "start", Target: "target"}}, f.Edges...)
	parent.Flow = &f
	c.Workflows = []Workflow{child, parent}
	return c, parent
}

func TestWorkflowCallIncludesInputsAndPreservesContextAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			c, parent := calledWorkflowConfig(t)
			if errs := ValidateWorkflow(parent, c, true); len(errs) > 0 {
				t.Fatal(errs)
			}
			inputs, err := RunInputs(c, Node{WorkflowID: parent.ID})
			if err != nil || len(inputs) != 1 {
				t.Fatal(inputs, err)
			}
			frozen, err := Freeze(c, Node{WorkflowID: parent.ID}, t.TempDir())
			if err != nil || len(frozen.Workflows) != 2 {
				t.Fatal(frozen, err)
			}
			var mu sync.Mutex
			commands := []string{}
			e, err := NewEngine(t.TempDir(), func(_ context.Context, s Server) (Session, error) {
				return deploymentSession{run: func(st Step) error {
					mu.Lock()
					defer mu.Unlock()
					commands = append(commands, st.Command)
					if fail && st.Command == "child" {
						return errors.New("child failed")
					}
					return nil
				}, publish: func(files []Artifact) error {
					if len(files) != 1 || files[0].Remote != "/opt/app/app" {
						t.Error(files)
					}
					return nil
				}}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			parent, _ = frozen.Workflow(parent.ID)
			err = e.workflow(ctx, frozen, parent, "run")
			if ctx.Err() != nil {
				t.Fatal("nested call deadlocked on parent host/resource reservation")
			}
			if (err != nil) != fail {
				t.Fatal(err)
			}
			want := "before,child,after"
			if fail {
				want = "before,child"
			}
			if strings.Join(commands, ",") != want {
				t.Fatal(commands)
			}
		})
	}
}

func TestWorkflowCallsRejectCyclesMissingReferencesAndUnorderedOverlap(t *testing.T) {
	c, parent := calledWorkflowConfig(t)
	parent.Steps[2].WorkflowID = parent.ID
	if errs := ValidateWorkflow(parent, c, false); !strings.Contains(strings.Join(errs, " "), "循环") {
		t.Fatal(errs)
	}
	parent.Steps[2].WorkflowID = "missing"
	if errs := ValidateWorkflow(parent, c, false); len(errs) == 0 {
		t.Fatal("missing reference accepted")
	}
	parent.Steps[2].WorkflowID = "child"
	parent.Steps = append(parent.Steps, Step{ID: "call2", Name: "second", Type: "workflow", WorkflowID: "child"})
	parent.Flow.Nodes = append(parent.Flow.Nodes, FlowNode{ID: "call2", Ref: "call2"})
	parent.Flow.Edges = append(parent.Flow.Edges, FlowEdge{Source: "start", Target: "call2"})
	if errs := ValidateWorkflow(parent, c, false); !strings.Contains(strings.Join(errs, " "), "先后顺序") {
		t.Fatal(errs)
	}
	c, parent = calledWorkflowConfig(t)
	c.Combinations = []Combination{{ID: "combo", Root: Node{Mode: "serial", Children: []Node{{WorkflowID: parent.ID}}}}}
	c.Workflows[1].Steps[2].WorkflowID = "combo"
	if err := ValidateNode(Node{WorkflowID: parent.ID}, c); err == nil {
		t.Fatal("cycle through composition accepted")
	}
	c, parent = calledWorkflowConfig(t)
	if err := os.Remove(c.Artifacts[0].Files[0].Local); err != nil {
		t.Fatal(err)
	}
	inputs, err := RunInputs(c, Node{WorkflowID: parent.ID})
	if err != nil || len(inputs) != 1 || inputs[0].Error == "" {
		t.Fatal("missing child asset not reported", inputs, err)
	}
}

func TestCalledParallelCompositionKeepsResourceSerialization(t *testing.T) {
	c, parent := calledWorkflowConfig(t)
	c.Servers[0].Concurrency = 0
	c.Combinations = []Combination{{ID: "combo", Root: Node{Mode: "parallel", Children: []Node{{WorkflowID: "child"}, {WorkflowID: "child"}}}}}
	parent = Workflow{ID: "parent", Name: "parent", Steps: []Step{{ID: "call", Type: "workflow", WorkflowID: "combo"}}}
	c.Workflows[1] = parent
	var mu sync.Mutex
	active, calls := 0, 0
	e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) {
		return deploymentSession{run: func(Step) error {
			mu.Lock()
			active++
			calls++
			if active != 1 {
				t.Error("shared child resource ran concurrently")
			}
			mu.Unlock()
			time.Sleep(10 * time.Millisecond)
			mu.Lock()
			active--
			mu.Unlock()
			return nil
		}, publish: func([]Artifact) error { return nil }}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := e.workflow(ctx, c, parent, "run"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}

func TestStartNodeFansOutAndRejectsDisconnectedOrIncomingEdges(t *testing.T) {
	f := Flow{Nodes: []FlowNode{{ID: "start", Ref: StartRef}, {ID: "a", Ref: "a"}, {ID: "b", Ref: "b"}}, Edges: []FlowEdge{{Source: "start", Target: "a"}, {Source: "start", Target: "b"}}}
	refs := map[string]bool{"a": true, "b": true}
	if err := ValidateFlow(f, refs); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	seen := map[string]bool{}
	if err := RunFlow(context.Background(), f, func(n FlowNode) error { mu.Lock(); seen[n.ID] = true; mu.Unlock(); return nil }, func(FlowNode) { t.Error("blocked") }); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen["start"] {
		t.Fatal(seen)
	}
	f.Edges = f.Edges[:1]
	if err := ValidateFlow(f, refs); err == nil {
		t.Fatal("disconnected node accepted")
	}
	f.Edges = append(f.Edges, FlowEdge{Source: "b", Target: "start"})
	if err := ValidateFlow(f, refs); err == nil {
		t.Fatal("incoming start accepted")
	}
}

func TestWorkflowCallRejectsHostSlotBarrierAndAllowsNewServerPhase(t *testing.T) {
	for _, concurrency := range []int{1, 2} {
		c, parent := calledWorkflowConfig(t)
		c.Servers = append(c.Servers, Server{ID: "b", Host: "b"}, Server{ID: "c", Host: "c"})
		parent.Steps[0].Targets.Servers = []string{"a", "b", "c"}
		parent.Steps[0].Targets.Concurrency = concurrency
		if errs := ValidateWorkflow(parent, c, false); !strings.Contains(strings.Join(errs, " "), "新的服务器区块") {
			t.Fatal("host-slot barrier accepted", errs)
		}
		next := Step{ID: "next", Type: "server", Targets: &Targets{Servers: []string{"a", "b", "c"}, Concurrency: concurrency}}
		parent.Steps = append(parent.Steps, next)
		parent.Flow.Nodes = append(parent.Flow.Nodes, FlowNode{ID: "next", Ref: "next"})
		for i := range parent.Flow.Edges {
			if parent.Flow.Edges[i].Source == "call" {
				parent.Flow.Edges[i].Target = "next"
			}
		}
		parent.Flow.Edges = append(parent.Flow.Edges, FlowEdge{Source: "next", Target: "after"})
		if errs := ValidateWorkflow(parent, c, false); len(errs) > 0 {
			t.Fatal(errs)
		}
		e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) {
			return deploymentSession{run: func(Step) error { return nil }, publish: func([]Artifact) error { return nil }}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		err = e.workflow(ctx, c, parent, "run")
		cancel()
		e.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestWorkflowCallCanJoinDifferentServerContexts(t *testing.T) {
	c, parent := calledWorkflowConfig(t)
	parent.Steps = []Step{parent.Steps[0], {ID: "second", Type: "server", Targets: &Targets{Servers: []string{"a"}, Concurrency: 1}}, parent.Steps[2]}
	parent.Flow = &Flow{Nodes: []FlowNode{{ID: "start", Ref: StartRef}, {ID: "target", Ref: "target"}, {ID: "second", Ref: "second"}, {ID: "call", Ref: "call"}}, Edges: []FlowEdge{{Source: "start", Target: "target"}, {Source: "start", Target: "second"}, {Source: "target", Target: "call"}, {Source: "second", Target: "call"}}}
	if errs := ValidateWorkflow(parent, c, false); len(errs) > 0 {
		t.Fatal(errs)
	}
	bindings, err := bindWorkflow(parent)
	if err != nil || bindings["call"].ID != "" {
		t.Fatal(bindings, err)
	}
}
