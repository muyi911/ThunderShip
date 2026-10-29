package main

import (
	"path/filepath"
	"shell_workflow/internal/core"
	"testing"
)

func TestTunnelRulesPersistWithoutReplacingServerOrCredentials(t *testing.T) {
	a := NewApp()
	a.dir = t.TempDir()
	a.config.Servers = []core.Server{{ID: "a", Name: "服务器 A", Host: "127.0.0.1", Port: 22, User: "test", Auth: "password", HasSecret: true}, {ID: "b", Name: "服务器 B"}}
	rule := core.TunnelRule{Name: "面板", Type: "local", BindHost: "127.0.0.1", BindPort: 8800, TargetHost: "127.0.0.1", TargetPort: 8800}
	before := a.config.Servers[0]
	rules, err := a.SaveTunnelRule("a", rule)
	if err != nil || len(rules) != 1 || rules[0].ID == "" {
		t.Fatal(rules, err)
	}
	if a.config.Servers[0] != before {
		t.Fatal("tunnel save changed server credentials")
	}
	oldConfig := a.config
	rule = rules[0]
	rule.TargetPort = 9900
	if _, err = a.SaveTunnelRule("a", rule); err != nil {
		t.Fatal(err)
	}
	if oldConfig.Tunnels["a"][0].TargetPort != 8800 {
		t.Fatal("save mutated the previous config snapshot")
	}
	saved, err := core.LoadConfig(filepath.Clean(a.dir))
	if err != nil || saved.Tunnels["a"][0].TargetPort != 9900 {
		t.Fatal("rules were not persisted", saved.Tunnels, err)
	}
	if empty, err := a.GetTunnelRules("b"); err != nil || empty == nil || len(empty) != 0 {
		t.Fatal("rules leaked to another server", empty, err)
	}
	if _, err := a.SaveTunnelRule("missing", rule); err == nil {
		t.Fatal("missing server accepted")
	}
	if _, err := a.StartTunnel("closed", rule.ID); err == nil {
		t.Fatal("closed connection accepted")
	}
	if _, err := a.DeleteTunnelRule("a", rule.ID); err != nil {
		t.Fatal(err)
	}
	if rules, _ := a.GetTunnelRules("a"); len(rules) != 0 {
		t.Fatal("delete left rule behind")
	}
	if a.config.Servers[0] != before {
		t.Fatal("delete changed server")
	}
}
