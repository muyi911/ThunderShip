package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
	"strings"
	"testing"
)

func testApp(t *testing.T) *App {
	t.Helper()
	a := NewApp()
	a.dir = t.TempDir()
	a.ctx = context.Background()
	var err error
	a.engine, err = core.NewEngine(a.dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func TestServerConcurrencyAllowsUnlimited(t *testing.T) {
	a := testApp(t)
	s := core.Server{Name: "Test", Host: "127.0.0.1", Port: 22, User: "test", Auth: "password"}
	for _, limit := range []int{0, 1, 64, 0} {
		s.Concurrency = limit
		c, err := a.SaveServer(s, "")
		if err != nil || len(c.Servers) != 1 || c.Servers[0].Concurrency != limit {
			t.Fatalf("limit %d: %+v, %v", limit, c.Servers, err)
		}
		s = c.Servers[0]
	}
	for _, limit := range []int{-1, 65} {
		s.Concurrency = limit
		if _, err := a.SaveServer(s, ""); err == nil {
			t.Fatalf("accepted invalid concurrency %d", limit)
		}
	}
	c, err := a.GetConfig()
	if err != nil || c.Servers[0].Concurrency != 0 {
		t.Fatal("invalid limit changed saved configuration", err)
	}
}

func TestResourceReferencesAndWorkflowVersions(t *testing.T) {
	a := testApp(t)
	c, err := a.SaveServer(core.Server{Name: "Test", Host: "127.0.0.1", Port: 22, User: "test", Auth: "password", Concurrency: 1}, "")
	if err != nil {
		t.Fatal(err)
	}
	sid := c.Servers[0].ID
	w := core.Workflow{Name: "Deploy", Servers: []string{sid}, Concurrency: 1, Steps: []core.Step{{ID: "step", Type: "command", Command: "true", Timeout: 3}}}
	c, err = a.SaveWorkflow(w)
	if err != nil {
		t.Fatal(err)
	}
	w = c.Workflows[0]
	if w.Version != 1 {
		t.Fatal(w.Version)
	}
	c, err = a.SaveWorkflow(w)
	if err != nil || c.Workflows[0].Version != 2 {
		t.Fatal(c, err)
	}
	if _, err = a.SaveWorkflow(w); err == nil {
		t.Fatal("stale version accepted")
	}
	c, err = a.SaveCombination(core.Combination{Name: "Together", Root: core.Node{Mode: "parallel", Children: []core.Node{{WorkflowID: w.ID}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Delete("workflow", w.ID); err == nil {
		t.Fatal("referenced workflow deleted")
	}
	if _, err = a.Delete("server", sid); err == nil {
		t.Fatal("referenced server deleted")
	}
	snapshot, _ := a.GetConfig()
	snapshot.Servers[0].Name = "changed"
	actual, _ := a.GetConfig()
	if actual.Servers[0].Name != "Test" {
		t.Fatal("config snapshot aliases live data")
	}
	if _, err = a.Delete("combination", c.Combinations[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Delete("workflow", w.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Delete("server", sid); err != nil {
		t.Fatal(err)
	}
}

func TestProviderModelsAndKeyStorage(t *testing.T) {
	a := testApp(t)
	const key = "disposable-app-test-key"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+key {
			t.Error("API credential was not forwarded")
		}
		switch r.URL.Path {
		case "/prefix/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/prefix/v1/chat/completions":
			var req struct {
				Model string `json:"model"`
			}
			if json.NewDecoder(r.Body).Decode(&req) != nil || req.Model != "test-model" {
				t.Error("wrong selected model")
			}
			fmt.Fprint(w, `{"choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}]}`)
		default:
			t.Error(r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c, err := a.SaveProvider(core.Provider{Name: "Test", BaseURL: srv.URL + "/prefix/v1", Model: "test-model", Mode: "auto", Models: []string{}}, key)
	if err != nil {
		t.Fatal(err)
	}
	p := c.Providers[0]
	defer core.DeleteSecret("provider/" + p.ID)
	models, err := a.ListModels(p.ID)
	if err != nil || len(models) != 1 || models[0] != "test-model" {
		t.Fatal(models, err)
	}
	if _, err = a.TestModel(p.ID); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(a.dir, "config.json"))
	if err != nil || strings.Contains(string(b), key) {
		t.Fatal("credential leaked to configuration", err)
	}
	p.BaseURL = "http://localhost:1/v1"
	if _, err = a.SaveProvider(p, ""); err == nil {
		t.Fatal("old key allowed for changed address")
	}
	if _, err = a.Delete("provider", p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = core.GetSecret("provider/" + p.ID); err == nil {
		t.Fatal("provider deletion left credential behind")
	}
}
