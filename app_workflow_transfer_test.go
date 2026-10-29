package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
	"strings"
	"testing"
)

func transferPackage(t *testing.T) core.WorkflowPackage {
	t.Helper()
	c := core.EmptyConfig()
	c.Servers = []core.Server{{ID: "foreign", Name: "foreign server"}}
	c.Workflows = []core.Workflow{{ID: "original", Name: "deploy", Servers: []string{"foreign"}, Concurrency: 1,
		Steps: []core.Step{{ID: "cmd", Type: "command", Command: "echo imported", Timeout: 60}}}}
	p, err := core.ExportWorkflowPackage(c, "original")
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportWorkflowsPersistsAtomicallyWithoutRunning(t *testing.T) {
	a := testApp(t)
	a.config.Servers = []core.Server{{ID: "local", Name: "local server"}}
	p := transferPackage(t)
	if _, err := a.ImportWorkflows(p, core.WorkflowBindings{}); err == nil {
		t.Fatal("import without explicit resource binding accepted")
	}
	if _, err := os.Stat(filepath.Join(a.dir, "config.json")); !os.IsNotExist(err) {
		t.Fatal("rejected import wrote configuration", err)
	}
	bindings := core.WorkflowBindings{Servers: map[string]string{"foreign": "local"}}
	c, err := a.ImportWorkflows(p, bindings)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := core.LoadConfig(a.dir)
	if err != nil || len(loaded.Workflows) != 1 || loaded.Workflows[0].ID == "original" || loaded.Workflows[0].Steps[0].Targets.Servers[0] != "local" {
		t.Fatal("new workflow not persisted with local resources", loaded, err)
	}
	if len(a.engine.List()) != 0 || len(c.Servers) != 1 || len(c.Artifacts) != 0 {
		t.Fatal("import started a run or created foreign resources")
	}
	before, _ := json.Marshal(a.config)
	blocked := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocked, nil, 0600); err != nil {
		t.Fatal(err)
	}
	a.dir = blocked
	if _, err := a.ImportWorkflows(p, bindings); err == nil {
		t.Fatal("save failure was ignored")
	}
	after, _ := json.Marshal(a.config)
	if string(before) != string(after) {
		t.Fatal("failed save changed in-memory configuration")
	}
}

func TestReadWorkflowPackageChecksFormatAndSize(t *testing.T) {
	p := transferPackage(t)
	b, _ := json.Marshal(p)
	filename := filepath.Join(t.TempDir(), "workflow.json")
	for _, test := range []struct {
		name string
		body []byte
		ok   bool
	}{
		{"valid", b, true},
		{"bom", append([]byte{0xef, 0xbb, 0xbf}, b...), true},
		{"invalid-json", []byte("{"), false},
		{"wrong-format", []byte(`{"format":"something-else","version":1}`), false},
		{"trailing-data", append(append([]byte(nil), b...), []byte(" {}")...), false},
		{"oversized", []byte(strings.Repeat(" ", (8<<20)+1)), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filename, test.body, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := readWorkflowPackage(filename)
			if (err == nil) != test.ok {
				t.Fatal("unexpected parse result", err)
			}
		})
	}
}
