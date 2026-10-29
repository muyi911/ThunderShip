package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSameServerMultipleDirectoriesShareMixedAssetSnapshots(t *testing.T) {
	c := EmptyConfig()
	c.Servers = []Server{{ID: "a", Host: "a"}}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"index.html", "assets/app.js"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0600); err != nil {
			t.Fatal(err)
		}
	}
	c.Artifacts = []ArtifactResource{{ID: "web", Name: "web", Files: []FileMapping{
		{ID: "index", Kind: "file", Local: filepath.Join(dir, "index.html")},
		{ID: "assets", Kind: "folder", Local: filepath.Join(dir, "assets")},
	}}}
	upload := Step{ID: "upload", Type: "upload", ArtifactID: "web", Timeout: 60}
	for _, root := range []string{"/home/work/one", "/home/work/two"} {
		for _, file := range c.Artifacts[0].Files {
			upload.Files = append(upload.Files, FileMapping{ID: ID(), Source: file.ID, Remote: root + "/" + filepath.Base(file.Local)})
		}
	}
	w := Workflow{ID: "w", Name: "two instances", Steps: []Step{
		{ID: "server", Type: "server", Targets: &Targets{Servers: []string{"a"}, Concurrency: 1}}, upload,
	}}
	c.Workflows = []Workflow{w}
	if errs := ValidateWorkflow(w, c, true); len(errs) > 0 {
		t.Fatal(errs)
	}
	inputs, err := RunInputs(c, Node{WorkflowID: w.ID})
	if err != nil || len(inputs) != 2 {
		t.Fatal("same asset requested once per destination", inputs, err)
	}
	frozen, err := Freeze(c, Node{WorkflowID: w.ID}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := frozen.Workflows[0].Steps[1].Files
	if len(files) != 4 || files[0].Local != files[2].Local || files[1].Local != files[3].Local {
		t.Fatal("snapshots not shared", files)
	}
	for i, remote := range []string{"/home/work/one/index.html", "/home/work/one/assets/app.js", "/home/work/two/index.html", "/home/work/two/assets/app.js"} {
		if files[i].Remote != remote {
			t.Fatal("destination hierarchy changed", files)
		}
	}
	resolved, err := ResolveUploads(frozen.Workflows[0], frozen)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planDeployment(frozen, resolved)
	if err != nil || len(plan.leases) != 1 || len(plan.leases[0].paths) < 4 {
		t.Fatal("same host destinations not reserved", plan.leases, err)
	}
	w.Steps[1].Files[2].Remote = w.Steps[1].Files[0].Remote
	if errs := ValidateWorkflow(w, c, true); len(errs) == 0 {
		t.Fatal("duplicate destination bypassed conflict checks")
	}
}
