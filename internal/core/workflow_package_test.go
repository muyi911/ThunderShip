package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func packageFixture(t *testing.T) (Config, WorkflowPackage) {
	t.Helper()
	c, w := branchWorkflow()
	for i := range c.Servers {
		c.Servers[i].Name = "server-" + c.Servers[i].ID
		c.Servers[i].Host = "private-host-" + c.Servers[i].ID
		c.Servers[i].KeyPath = `C:\private\key.pem`
		c.Servers[i].HasSecret = true
	}
	c.Artifacts[0].Files[0].Local = `C:\private\build\program`
	c.Artifacts[0].Files = append(c.Artifacts[0].Files,
		FileMapping{ID: "folder", Name: "assets", Kind: "folder", Local: `C:\private\build\assets`},
		FileMapping{ID: "index", Name: "index.html", Kind: "file", Local: `C:\private\build\index.html`},
		FileMapping{ID: "unused", Name: "unused-secret", Kind: "file", Local: `C:\private\unused-secret`})
	w.Steps[1].Files = append(w.Steps[1].Files,
		FileMapping{ID: "folder-map", Source: "folder", Remote: "/srv/front", IncludeFolder: true},
		FileMapping{ID: "index-map", Source: "index", Remote: "/srv/front/index.html"})
	w.Steps[2].CommandMode, w.Steps[2].Commands = "lines", []string{"echo one", "echo two"}
	w.Flow.Nodes[0].X, w.Flow.Nodes[0].Y = 123, 456
	c.Workflows[0] = w
	c.Workflows = append(c.Workflows, Workflow{ID: "parent", Name: "parent", Steps: []Step{{ID: "call", Name: "call", Type: "workflow", WorkflowID: w.ID}}})
	c.Combinations = []Combination{{ID: "composition", Name: "composition", Root: Node{Graph: &Flow{
		Nodes: []FlowNode{{ID: "start", Ref: StartRef}, {ID: "parent-node", Ref: "parent", X: 12, Y: 34}},
		Edges: []FlowEdge{{Source: "start", Target: "parent-node"}},
	}}}}
	p, err := ExportWorkflowPackage(c, "composition")
	if err != nil {
		t.Fatal(err)
	}
	return c, p
}

func packageBindings() (Config, WorkflowBindings) {
	c := EmptyConfig()
	c.Servers = []Server{{ID: "local-a", Name: "local A", Host: "local-a"}, {ID: "local-b", Name: "local B", Host: "local-b"}}
	c.Artifacts = []ArtifactResource{{ID: "local-asset", Name: "local build", Files: []FileMapping{
		{ID: "local-bin", Name: "program", Kind: "file", Local: "/local/program"},
		{ID: "local-folder", Name: "assets", Kind: "folder", Local: "/local/assets"},
		{ID: "local-index", Name: "index.html", Kind: "file", Local: "/local/index.html"},
	}}}
	bindings := WorkflowBindings{
		Servers:   map[string]string{"a": "local-a", "b": "local-b"},
		Artifacts: map[string]ArtifactBinding{"asset": {ID: "local-asset", Files: map[string]string{"bin": "local-bin", "folder": "local-folder", "index": "local-index"}}},
	}
	return c, bindings
}

func TestWorkflowPackageRoundTripAndIsolation(t *testing.T) {
	original, p := packageFixture(t)
	if len(p.Workflows) != 2 || len(p.Combinations) != 1 || len(p.Servers) != 2 || len(p.Artifacts) != 1 || len(p.Artifacts[0].Files) != 3 {
		t.Fatal("missing dependencies or included unused resources", p)
	}
	b, _ := json.Marshal(p)
	for _, private := range []string{"private-host", "private", "unused-secret", "hasSecret", "keyPath", "providers"} {
		if strings.Contains(string(b), private) {
			t.Fatalf("export leaked %q", private)
		}
	}
	if original.Artifacts[0].Files[0].Local == "" || original.Workflows[0].Steps[1].ArtifactID != "asset" {
		t.Fatal("export mutated live resources")
	}
	c, bindings := packageBindings()
	c.Workflows = []Workflow{{ID: "w", Name: "branches", Version: 9}}
	c.Combinations = []Combination{{ID: "composition", Name: "composition"}}
	before, _ := json.Marshal(c)
	imported, err := ImportWorkflowPackage(c, p, bindings)
	if err != nil {
		t.Fatal(err)
	}
	leaf, parent := imported.Workflows[2], imported.Workflows[1]
	if leaf.ID == "w" || parent.ID == "parent" || leaf.Version != 1 || leaf.Name != "branches - 导入 1" || imported.Workflows[0].Version != 9 {
		t.Fatal("import overwrote or reused old identities", imported.Workflows)
	}
	if parent.Steps[0].WorkflowID != leaf.ID || imported.Combinations[1].Root.Graph.Nodes[1].Ref != parent.ID {
		t.Fatal("child workflow or composition reference was not rebound")
	}
	if !reflect.DeepEqual(leaf.Flow, original.Workflows[0].Flow) || leaf.Steps[0].Targets.Servers[0] != "local-a" {
		t.Fatal("graph or server bindings changed incorrectly", leaf)
	}
	files := leaf.Steps[1].Files
	if leaf.Steps[1].ArtifactID != "local-asset" || files[0].Source != "local-bin" || files[1].Source != "local-folder" || !files[1].IncludeFolder || files[2].Source != "local-index" || files[2].Remote != "/srv/front/index.html" {
		t.Fatal("mixed file/folder destinations or bindings lost", files)
	}
	if leaf.Steps[2].CommandMode != "lines" || !reflect.DeepEqual(leaf.Steps[2].Commands, []string{"echo one", "echo two"}) {
		t.Fatal("command mode or commands lost")
	}
	after, _ := json.Marshal(c)
	packageAfter, _ := json.Marshal(p)
	if string(before) != string(after) || string(b) != string(packageAfter) {
		t.Fatal("import mutated its inputs")
	}
	if !reflect.DeepEqual(imported.Servers, c.Servers) || !reflect.DeepEqual(imported.Artifacts, c.Artifacts) {
		t.Fatal("import changed local resources")
	}
}

func TestWorkflowPackageRejectsInvalidReferencesAndBindings(t *testing.T) {
	_, original := packageFixture(t)
	for _, scenario := range []string{"version", "null-resources", "duplicate-workflow", "duplicate-server", "duplicate-file", "missing-child", "cycle", "local-path", "server-binding", "asset-binding", "file-binding", "wrong-kind"} {
		t.Run(scenario, func(t *testing.T) {
			p := copyPackage(original)
			c, bindings := packageBindings()
			switch scenario {
			case "version":
				p.Version++
			case "null-resources":
				p.Artifacts = nil
			case "duplicate-workflow":
				p.Workflows = append(p.Workflows, p.Workflows[0])
			case "duplicate-server":
				p.Servers = append(p.Servers, p.Servers[0])
			case "duplicate-file":
				p.Artifacts[0].Files = append(p.Artifacts[0].Files, p.Artifacts[0].Files[0])
			case "missing-child":
				p.Workflows[0].Steps[0].WorkflowID = "external-workflow"
			case "cycle":
				p.Workflows[0].Steps[0].WorkflowID = "parent"
			case "local-path":
				p.Workflows[1].Steps[1].Files[0].Local = "injected-local-path"
			case "server-binding":
				delete(bindings.Servers, "a")
			case "asset-binding":
				delete(bindings.Artifacts, "asset")
			case "file-binding":
				delete(bindings.Artifacts["asset"].Files, "bin")
			case "wrong-kind":
				bindings.Artifacts["asset"].Files["folder"] = "local-index"
			}
			before, _ := json.Marshal(c)
			if _, err := ImportWorkflowPackage(c, p, bindings); err == nil {
				t.Fatal("invalid import accepted")
			}
			after, _ := json.Marshal(c)
			if string(before) != string(after) {
				t.Fatal("invalid import changed existing resources")
			}
		})
	}
}

func TestWorkflowPackageConvertsLegacyInlineUpload(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "assets")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	c := EmptyConfig()
	c.Servers = []Server{{ID: "server", Name: "old server"}}
	c.Workflows = []Workflow{{ID: "legacy", Name: "legacy", Servers: []string{"server"}, Concurrency: 1,
		Steps: []Step{{ID: "upload", Name: "upload", Type: "upload", Timeout: 60,
			Files: []FileMapping{{ID: "folder", Local: folder, Remote: "/front", IncludeFolder: true}}}}}}
	p, err := ExportWorkflowPackage(c, "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Artifacts) != 1 || p.Artifacts[0].Files[0].Name != "assets" || p.Artifacts[0].Files[0].Kind != "folder" || p.Workflows[0].Steps[1].Files[0].Local != "" {
		t.Fatal("legacy inline upload was not converted", p)
	}
}
