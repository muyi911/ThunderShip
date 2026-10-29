package main

import (
	"shell_workflow/internal/core"
	"testing"
)

func TestSaveAndDeleteReferencedWorkflow(t *testing.T) {
	a := testApp(t)
	c, err := a.SaveServer(core.Server{Name: "test", Host: "127.0.0.1", Port: 22, User: "test", Auth: "password"}, "")
	if err != nil {
		t.Fatal(err)
	}
	child := core.Workflow{Name: "child", Steps: []core.Step{
		{ID: "server", Type: "server", Targets: &core.Targets{Servers: []string{c.Servers[0].ID}, Concurrency: 1}},
		{ID: "command", Type: "command", Command: "true", Timeout: 10},
	}}
	c, err = a.SaveWorkflow(child)
	if err != nil {
		t.Fatal(err)
	}
	child = c.Workflows[0]
	parent := core.Workflow{Name: "caller", Steps: []core.Step{{ID: "call", Type: "workflow", WorkflowID: child.ID}}, Flow: &core.Flow{Nodes: []core.FlowNode{{ID: "start", Ref: core.StartRef}, {ID: "call", Ref: "call"}}, Edges: []core.FlowEdge{{Source: "start", Target: "call"}}}}
	c, err = a.SaveWorkflow(parent)
	if err != nil {
		t.Fatal(err)
	}
	parent = c.Workflows[1]
	if _, err = a.Delete("workflow", child.ID); err == nil {
		t.Fatal("called workflow deleted")
	}
	child.Steps = append(child.Steps, core.Step{ID: "call-back", Type: "workflow", WorkflowID: parent.ID})
	if _, err = a.SaveWorkflow(child); err == nil {
		t.Fatal("cycle introduced by editing callee")
	}
	if _, err = a.Delete("workflow", parent.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Delete("workflow", child.ID); err != nil {
		t.Fatal(err)
	}
}
