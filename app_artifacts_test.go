package main

import (
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
	"testing"
)

func TestArtifactPathsPersistAndReferencesPreventDestructiveEdits(t *testing.T) {
	a := testApp(t)
	asset := core.ArtifactResource{Name: "program", Files: []core.FileMapping{{ID: "program", Name: "service", Kind: "file", Local: filepath.Join(t.TempDir(), "not-built-yet")}}}
	c, err := a.SaveArtifact(asset)
	if err != nil {
		t.Fatal(err)
	}
	asset = c.Artifacts[0]
	c, err = a.SaveServer(core.Server{Name: "test", Host: "127.0.0.1", Port: 22, User: "test", Auth: "password"}, "")
	if err != nil {
		t.Fatal(err)
	}
	w := core.Workflow{Name: "deploy", Steps: []core.Step{
		{ID: "server", Name: "target", Type: "server", Targets: &core.Targets{Servers: []string{c.Servers[0].ID}, Concurrency: 1}},
		{ID: "upload", Name: "upload", Type: "upload", ArtifactID: asset.ID, Timeout: 60, Files: []core.FileMapping{{ID: "mapping", Source: "program", Remote: "/home/work/app"}}},
	}}
	c, err = a.SaveWorkflow(w)
	if err != nil {
		t.Fatal(err)
	}
	w = c.Workflows[0]
	inputs, err := a.RunInputs(core.Node{WorkflowID: w.ID})
	if err != nil || len(inputs) != 1 || inputs[0].Error == "" {
		t.Fatal(inputs, err)
	}
	if _, err = a.Delete("artifact", asset.ID); err == nil {
		t.Fatal("referenced asset deleted")
	}
	bad := asset
	bad.Files = []core.FileMapping{{ID: "different", Kind: "file"}}
	if _, err = a.SaveArtifact(bad); err == nil {
		t.Fatal("referenced input removed")
	}
	file := filepath.Join(t.TempDir(), "service-new-location")
	if err = os.WriteFile(file, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	asset.Files[0].Local = file
	if _, err = a.SaveArtifact(asset); err != nil {
		t.Fatal(err)
	}
	loaded, err := core.LoadConfig(a.dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Artifacts[0].Files[0].Local != file || loaded.Workflows[0].Version != w.Version || loaded.Workflows[0].Steps[1].Files[0].Local != "" {
		t.Fatal("asset update changed workflow or was not persisted", loaded)
	}
	inputs, err = a.RunInputs(core.Node{WorkflowID: w.ID})
	if err != nil || inputs[0].Local != file || inputs[0].Error != "" {
		t.Fatal(inputs, err)
	}
	if err = os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if _, err = a.StartRun(core.Node{WorkflowID: w.ID}, "missing"); err == nil {
		t.Fatal("run with missing asset started")
	}
	if len(a.engine.List()) != 0 {
		t.Fatal("missing asset created a run")
	}
}
