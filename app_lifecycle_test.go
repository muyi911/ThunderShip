package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"shell_workflow/internal/core"
)

func TestServiceLifecyclePreservesConfigAndCancelsWork(t *testing.T) {
	a := NewApp()
	a.dir = t.TempDir()
	config := core.EmptyConfig()
	config.Servers = []core.Server{{ID: "saved", Name: "Saved server", Host: "127.0.0.1", Port: 22, User: "test", Auth: "password"}}
	filename := filepath.Join(a.dir, "config.json")
	if err := core.WriteJSON(filename, config); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer a.ServiceShutdown()
	loaded, err := a.GetConfig()
	if err != nil || len(loaded.Servers) != 1 || loaded.Servers[0].ID != "saved" {
		t.Fatalf("startup lost config: %+v, %v", loaded, err)
	}
	if err := a.ServiceShutdown(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(a.ctx.Err(), context.Canceled) {
		t.Fatal("shutdown did not cancel work")
	}
	after, err := os.ReadFile(filename)
	if err != nil || string(before) != string(after) {
		t.Fatal("lifecycle changed saved config", err)
	}
}

func TestDialogCancellationDoesNotHideFailures(t *testing.T) {
	if dialogError(nil) != nil || dialogError(errors.New("cancelled by user")) != nil {
		t.Fatal("cancelled picker should return no error")
	}
	failure := errors.New("access denied")
	if dialogError(failure) != failure {
		t.Fatal("dialog failure was swallowed")
	}
}
