package main

import (
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
	"strings"
	"testing"
)

func TestMixedLocalSelectionExpandsOneDeploymentBatch(t *testing.T) {
	dir := t.TempDir()
	folder := filepath.Join(dir, "assets")
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(dir, "index.html")
	js := filepath.Join(folder, "app.js")
	for _, name := range []string{index, js} {
		if err := os.WriteFile(name, []byte("build"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	a := NewApp()
	listing, err := a.ListLocalDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if listing.Path != dir || listing.Parent != filepath.Dir(dir) || len(listing.Entries) != 2 || !listing.Entries[0].Directory || listing.Entries[0].Name != "assets" || listing.Entries[1].Name != "index.html" {
		t.Fatalf("mixed listing: %+v", listing)
	}
	files, err := a.ResolveLocalPaths([]string{index, folder, filepath.Join(dir, ".", "index.html")})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 || files[0].Kind != "file" || files[1].Kind != "folder" || files[0].Name != "index.html" || files[1].Name != "assets" || files[0].ID == files[1].ID {
		t.Fatalf("mixed selection lost identity or type: %+v", files)
	}
	for i := range files {
		files[i].Remote = "/var/www/" + files[i].Name
	}
	expanded, err := core.Expand(files)
	if err != nil {
		t.Fatal(err)
	}
	if len(expanded) != 2 || expanded[0].Remote != "/var/www/index.html" || expanded[1].Remote != "/var/www/assets/app.js" {
		t.Fatalf("batch hierarchy lost: %+v", expanded)
	}
	if _, err := a.ListLocalDirectory(index); err != nil {
		t.Fatal("saved file should locate its parent", err)
	}
	if err := os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if files, err := a.ResolveLocalPaths([]string{folder, index}); err == nil || files != nil {
		t.Fatal("missing file silently accepted or partially added", files, err)
	}
	if _, err := a.ListLocalDirectory(index); err == nil {
		t.Fatal("missing directory silently accepted")
	}
	if _, err := a.ResolveLocalPaths([]string{""}); err == nil {
		t.Fatal("empty path accepted")
	}
	files, err = a.ResolveLocalPaths(nil)
	if err != nil || files == nil || len(files) != 0 {
		t.Fatal("cancelled native dialog changed", files, err)
	}
}

func TestRemoteTextEndpointsRejectInvalidPathsAndClosedConnections(t *testing.T) {
	a := NewApp()
	for _, remote := range []string{"", "relative.txt", "/a/../secret", "/a\nfile", "/a\x00file", "/a\\file"} {
		if _, err := a.ReadRemoteText("closed", remote, false); err == nil || err.Error() != "请输入有效的远端绝对路径" {
			t.Fatal("invalid read path was not rejected before connecting", remote, err)
		}
		if _, err := a.SaveRemoteText("closed", remote, "content", "version", false); err == nil || err.Error() != "请输入有效的远端绝对路径" {
			t.Fatal("invalid save path was not rejected before connecting", remote, err)
		}
	}
	if _, err := a.SaveRemoteText("closed", "/config.env", "content", "version", false); err == nil || err.Error() != "连接已关闭" {
		t.Fatal("save accepted a closed connection", err)
	}
}

func TestRemoteFileActionsValidateBeforeConnecting(t *testing.T) {
	a := NewApp()
	if err := a.ManageRemoteFile("closed", core.RemoteFileOperation{Action: "delete", Source: "/"}, false); err == nil || err.Error() == "连接已关闭" {
		t.Fatal("root path should be rejected before connecting", err)
	}
	if err := a.ManageRemoteFile("closed", core.RemoteFileOperation{Action: "mkdir", Source: "/new"}, false); err == nil || err.Error() != "连接已关闭" {
		t.Fatal("operation accepted a closed connection", err)
	}
}

func TestDirectUploadRejectsInvalidAndAmbiguousSourcesBeforeConnecting(t *testing.T) {
	a := NewApp()
	if err := a.UploadRemotePaths("closed", "relative", nil, false, false); err == nil {
		t.Fatal("invalid destination accepted")
	}
	if err := a.UploadRemotePaths("closed", "/upload", []string{filepath.Join(t.TempDir(), "missing")}, false, false); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatal("missing source accepted", err)
	}
	paths := []string{}
	for i := 0; i < 2; i++ {
		name := filepath.Join(t.TempDir(), "same.txt")
		if err := os.WriteFile(name, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, name)
	}
	if err := a.UploadRemotePaths("closed", "/upload", paths, false, false); err == nil || !strings.Contains(err.Error(), "同名") {
		t.Fatal("ambiguous batch accepted", err)
	}
	if _, err := a.CompleteTerminalPath("closed", "relative", "a", false); err == nil {
		t.Fatal("invalid completion destination accepted")
	}
}
