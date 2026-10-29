package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFolderUploadModesAndSnapshot(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "assets")
	if err := os.MkdirAll(filepath.Join(folder, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"index.html", "assets/nested/app.js"} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(file), 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, include := range []bool{false, true} {
		t.Run(map[bool]string{false: "contents", true: "folder"}[include], func(t *testing.T) {
			mapping := FileMapping{ID: "folder", Source: "folder", Local: folder, Remote: "/front", IncludeFolder: include}
			expected := "/front/nested/app.js"
			if include {
				expected = "/front/assets/nested/app.js"
			}
			files, err := Expand([]FileMapping{mapping})
			if err != nil || len(files) != 1 || files[0].Remote != expected {
				t.Fatalf("preview: %+v %v", files, err)
			}
			c := EmptyConfig()
			c.Servers = []Server{{ID: "a", Host: "a"}}
			c.Artifacts = []ArtifactResource{{ID: "web", Name: "web", Files: []FileMapping{
				{ID: "folder", Name: "display alias", Kind: "folder", Local: folder},
				{ID: "html", Kind: "file", Local: filepath.Join(dir, "index.html")},
			}}}
			mapping.Local = ""
			w := Workflow{ID: "w", Name: "web", Steps: []Step{
				{ID: "server", Type: "server", Targets: &Targets{Servers: []string{"a"}, Concurrency: 1}},
				{ID: "upload", Type: "upload", ArtifactID: "web", Timeout: 60, Files: []FileMapping{
					mapping, {ID: "html", Source: "html", Remote: "/front/index.html"},
				}},
			}}
			c.Workflows = []Workflow{w}
			if errs := ValidateWorkflow(w, c, true); len(errs) != 0 {
				t.Fatal(errs)
			}
			frozen, err := Freeze(c, Node{WorkflowID: "w"}, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			upload := frozen.Workflows[0].Steps[1]
			if len(upload.Files) != 2 || upload.Files[0].Remote != expected || upload.Files[1].Remote != "/front/index.html" {
				t.Fatalf("snapshot changed paths or lost index.html: %+v", upload.Files)
			}
			if upload.Files[0].IncludeFolder {
				t.Fatal("frozen file must not append folder name again")
			}
			lock := "/front"
			if include {
				lock = "/front/assets"
			}
			if upload.LockPaths[0] != lock {
				t.Fatal("wrong folder lock", upload.LockPaths)
			}
			resolved, err := ResolveUploads(w, c)
			if err != nil {
				t.Fatal(err)
			}
			plan, err := planDeployment(c, resolved)
			if err != nil || plan.leases[0].paths[0] != lock {
				t.Fatal("unfrozen lock mismatch", plan.leases, err)
			}
			data, err := json.Marshal(w)
			if err != nil {
				t.Fatal(err)
			}
			var loaded Workflow
			if err = json.Unmarshal(data, &loaded); err != nil || loaded.Steps[1].Files[0].IncludeFolder != include {
				t.Fatal("folder mode not persisted", err)
			}
		})
	}
	files, err := Expand([]FileMapping{{Local: folder, Remote: "/", IncludeFolder: true}})
	if err != nil || files[0].Remote != "/assets/nested/app.js" {
		t.Fatal(files, err)
	}
	if _, err = Expand([]FileMapping{{Local: filepath.Join(dir, "index.html"), Remote: "/front", IncludeFolder: true}}); err == nil {
		t.Fatal("ordinary file accepted folder mode")
	}
	if _, err = Expand([]FileMapping{{Local: folder, Remote: "/front/../etc", IncludeFolder: true}}); err == nil {
		t.Fatal("folder mode bypassed destination validation")
	}
	if _, err = Expand([]FileMapping{
		{Local: folder, Remote: "/front", IncludeFolder: true},
		{Local: filepath.Join(folder, "nested", "app.js"), Remote: "/front/assets/nested/app.js"},
	}); err == nil {
		t.Fatal("folder mode bypassed destination conflicts")
	}
}
