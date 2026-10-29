package core

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestCommandLinesStopOnFailure(t *testing.T) {
	var mu sync.Mutex
	commands := []string{}
	e, err := NewEngine(t.TempDir(), func(context.Context, Server) (Session, error) { return fakeSession{&mu, &commands}, nil })
	if err != nil {
		t.Fatal(err)
	}
	st := Step{Type: "command", CommandMode: "lines", Commands: []string{"first", "fail", "must-not-run"}, Command: "inactive-script", Timeout: 5}
	var logs []string
	err = e.step(context.Background(), Server{}, st, func(_, message, _ string, _, _ int64) { logs = append(logs, message) })
	if err == nil || !strings.Contains(err.Error(), "第 2 条") || !reflect.DeepEqual(commands, []string{"first", "fail"}) {
		t.Fatal(commands, err)
	}
	if !strings.Contains(strings.Join(logs, " "), "命令 1/3 · 成功") {
		t.Fatal(logs)
	}
	commands = nil
	st.CommandMode = "script"
	if err = e.step(context.Background(), Server{}, st, func(string, string, string, int64, int64) {}); err != nil || !reflect.DeepEqual(commands, []string{"inactive-script"}) {
		t.Fatal(commands, err)
	}
	commands = nil
	st.CommandMode = ""
	if err = e.step(context.Background(), Server{}, st, func(string, string, string, int64, int64) {}); err != nil || !reflect.DeepEqual(commands, []string{"inactive-script"}) {
		t.Fatal("legacy script changed", commands, err)
	}
	commands = nil
	st.CommandMode = "lines"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err = e.step(ctx, Server{}, st, func(string, string, string, int64, int64) {}); !errors.Is(err, context.Canceled) || len(commands) != 0 {
		t.Fatal("cancelled list ran", commands, err)
	}
}

func TestCommandModeValidation(t *testing.T) {
	c := EmptyConfig()
	c.Servers = []Server{{ID: "s"}}
	for _, tc := range []struct {
		name, mode string
		commands   []string
		valid      bool
	}{
		{"legacy", "", nil, true},
		{"script", "script", []string{""}, true},
		{"lines", "lines", []string{"echo first", "echo second"}, true},
		{"empty", "lines", nil, false},
		{"blank", "lines", []string{"echo first", " "}, false},
		{"multiline", "lines", []string{"echo first\necho second"}, false},
		{"nul", "lines", []string{"echo \x00"}, false},
		{"unknown", "other", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := workflow("w", "echo script")
			w.Steps[0].CommandMode = tc.mode
			w.Steps[0].Commands = tc.commands
			errs := ValidateWorkflow(w, c, false)
			if (len(errs) == 0) != tc.valid {
				t.Fatal(errs)
			}
		})
	}
	w := workflow("w", "true")
	w.Steps[0].Type = "health"
	w.Steps[0].Interval = 1
	w.Steps[0].CommandMode = "lines"
	w.Steps[0].Commands = []string{"true"}
	if errs := ValidateWorkflow(w, c, false); len(errs) == 0 {
		t.Fatal("health line mode accepted")
	}
}
