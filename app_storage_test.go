package main

import (
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
	"testing"
)

func TestDataDirectoryMigration(t *testing.T) {
	for _, emptyTarget := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent destination", true: "empty destination"}[emptyTarget], func(t *testing.T) {
			base := t.TempDir()
			old := filepath.Join(base, "ShellWorkflow")
			c := core.EmptyConfig()
			c.Servers = []core.Server{{ID: "keep-credential-id", Name: "原服务器", HasSecret: true}}
			if err := core.WriteJSON(filepath.Join(old, "config.json"), c); err != nil {
				t.Fatal(err)
			}
			files := map[string]string{
				"runs/run-1/events.jsonl":     "original event\n",
				"runs/run-1/files/index.html": "<html>原构建物</html>",
			}
			for name, content := range files {
				path := filepath.Join(old, name)
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if emptyTarget {
				if err := os.Mkdir(filepath.Join(base, "ThunderShip"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			dir, err := migrateDataDirectory(base)
			if err != nil {
				t.Fatal(err)
			}
			original, err := os.ReadFile(filepath.Join(old, "config.json"))
			if err != nil {
				t.Fatal(err)
			}
			files["config.json"] = string(original)
			for name, want := range files {
				for _, root := range []string{old, dir} {
					got, err := os.ReadFile(filepath.Join(root, name))
					if err != nil || string(got) != want {
						t.Fatalf("%s: content changed or missing: %v", filepath.Join(root, name), err)
					}
				}
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"servers":[]}`), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := migrateDataDirectory(base); err != nil {
				t.Fatal(err)
			}
			got, _ := os.ReadFile(filepath.Join(dir, "config.json"))
			if string(got) != `{"servers":[]}` {
				t.Fatal("a later startup overwrote the new configuration")
			}
		})
	}
}

func TestDataDirectoryMigrationPreservesConflicts(t *testing.T) {
	for _, invalidOld := range []bool{false, true} {
		base := t.TempDir()
		old := filepath.Join(base, "ShellWorkflow")
		if err := core.WriteJSON(filepath.Join(old, "config.json"), core.EmptyConfig()); err != nil {
			t.Fatal(err)
		}
		if invalidOld {
			if err := os.WriteFile(filepath.Join(old, "config.json"), []byte("broken JSON"), 0600); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := core.WriteJSON(filepath.Join(base, "ThunderShip", "existing.json"), "keep"); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := migrateDataDirectory(base); err == nil {
			t.Fatal("expected migration to stop instead of replacing conflicting data")
		}
		if _, err := os.Stat(filepath.Join(base, "ThunderShip", "config.json")); !os.IsNotExist(err) {
			t.Fatalf("published a failed migration: %v", err)
		}
		entries, err := os.ReadDir(base)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Name() != "ShellWorkflow" && entry.Name() != "ThunderShip" {
				t.Fatalf("left behind staging data: %s", entry.Name())
			}
		}
	}
}

func TestFreshAndExplicitDataDirectory(t *testing.T) {
	base := t.TempDir()
	dir, err := migrateDataDirectory(base)
	if err != nil || dir != filepath.Join(base, "ThunderShip") {
		t.Fatalf("new installation: %q, %v", dir, err)
	}
	custom := filepath.Join(t.TempDir(), "自定义数据")
	t.Setenv("SHELL_WORKFLOW_DATA", custom)
	got, err := prepareDataDirectory()
	if err != nil || got != custom {
		t.Fatalf("explicit directory ignored: %q, %v", got, err)
	}
	if _, err := os.Stat(custom); !os.IsNotExist(err) {
		t.Fatal("explicit data selection unexpectedly copied or created data")
	}
}
