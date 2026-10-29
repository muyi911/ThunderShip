package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"shell_workflow/internal/core"
	"testing"
)

func TestResourceOrderPersistsWithoutChangingResources(t *testing.T) {
	a := testApp(t)
	a.config.Servers = []core.Server{{ID: "a", Name: "A", Group: "game"}, {ID: "b", Name: "B", Group: "other"}, {ID: "c", Name: "C", Group: "game"}}
	a.config.Artifacts = []core.ArtifactResource{{ID: "file-a"}, {ID: "file-b"}}
	a.config.Workflows = []core.Workflow{{ID: "w-a", Version: 7}, {ID: "w-b", Version: 3}}
	a.config.Combinations = []core.Combination{{ID: "combined"}}
	original, _ := a.GetConfig()
	for _, move := range []struct {
		kind, id, before string
		want             []string
	}{
		{"servers", "c", "a", []string{"c", "a", "b"}},
		{"servers", "c", "", []string{"a", "b", "c"}},
		{"servers", "a", "c", []string{"b", "a", "c"}}, // Moving in a group preserves hidden resources.
		{"artifacts", "file-b", "file-a", []string{"file-b", "file-a"}},
		{"workflows", "combined", "w-b", []string{"w-a", "combined", "w-b"}},
	} {
		c, err := a.MoveResource(move.kind, move.id, move.before)
		if err != nil || !reflect.DeepEqual(c.ResourceOrder[move.kind], move.want) {
			t.Fatal(move, c.ResourceOrder, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(a.dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved core.Config
	if err := json.Unmarshal(b, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.ResourceOrder) != 3 {
		t.Fatal("orders were overwritten", saved.ResourceOrder)
	}
	saved.ResourceOrder = nil
	if !reflect.DeepEqual(saved, original) {
		t.Fatal("display ordering changed resource data or workflow versions")
	}
	// Deleted IDs are ignored, and newly added resources remain available.
	a.config.Servers = []core.Server{{ID: "b"}, {ID: "c"}, {ID: "new"}}
	c, err := a.MoveResource("servers", "new", "b")
	if err != nil || !reflect.DeepEqual(c.ResourceOrder["servers"], []string{"new", "b", "c"}) {
		t.Fatal("stale order lost new resources", c.ResourceOrder, err)
	}
}

func TestResourceMoveRejectsInvalidRequestsAndFailedWrites(t *testing.T) {
	a := testApp(t)
	a.config.Servers = []core.Server{{ID: "a"}, {ID: "b"}}
	a.config.ResourceOrder = map[string][]string{"servers": {"a", "b"}}
	for _, args := range [][3]string{{"providers", "a", "b"}, {"servers", "missing", "a"}, {"servers", "a", "missing"}} {
		if _, err := a.MoveResource(args[0], args[1], args[2]); err == nil {
			t.Fatal("invalid move accepted", args)
		}
	}
	if err := os.Mkdir(filepath.Join(a.dir, "config.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := a.MoveResource("servers", "b", "a"); err == nil {
		t.Fatal("failed save reported success")
	}
	if !reflect.DeepEqual(a.config.ResourceOrder["servers"], []string{"a", "b"}) {
		t.Fatal("failed save changed in-memory order")
	}
}
