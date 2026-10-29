package main

import (
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
	"testing"
)

func TestUploadProtocolSettingsPersistAndPreserveResources(t *testing.T) {
	a := testApp(t)
	a.config.Servers = []core.Server{{ID: "existing-server"}}
	a.config.Workflows = []core.Workflow{{ID: "existing-workflow"}}
	for _, protocol := range []string{"scp", "sftp"} {
		if _, err := a.SaveSettings(4, "dark", protocol); err != nil {
			t.Fatal(err)
		}
		c, err := core.LoadConfig(a.dir)
		if err != nil || c.UploadProtocol != protocol || len(c.Servers) != 1 || len(c.Workflows) != 1 {
			t.Fatal("upload preference failed to round-trip or changed resources", c, err)
		}
	}
	if _, err := a.SaveSettings(4, "dark", "unknown"); err == nil || a.config.UploadProtocol != "sftp" {
		t.Fatal("invalid upload protocol changed the configuration", err)
	}
	if err := os.WriteFile(filepath.Join(a.dir, "config.json"), []byte(`{"theme":"dark","servers":[{"id":"existing-server"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	legacy, err := core.LoadConfig(a.dir)
	if err != nil || legacy.UploadProtocol != "sftp" || len(legacy.Servers) != 1 {
		t.Fatal("legacy configuration lost the default or existing servers", legacy, err)
	}
}
