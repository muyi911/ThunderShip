package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDataDirectory(t *testing.T) {
	a := NewApp()
	if _, err := a.GetDataDirectory(); err == nil {
		t.Fatal("accepted uninitialized directory")
	}
	a.dir = filepath.Join(t.TempDir(), "数据 空间")
	if err := os.Mkdir(a.dir, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := a.GetDataDirectory()
	if err != nil || !filepath.IsAbs(dir) {
		t.Fatalf("directory: %q, %v", dir, err)
	}
	want, _ := os.Stat(a.dir)
	got, err := os.Stat(dir)
	if err != nil || !os.SameFile(want, got) {
		t.Fatalf("resolved to a different directory: %q, %v", dir, err)
	}
	a.dir = filepath.Join(a.dir, "missing")
	if err := a.OpenDataDirectory(); err == nil {
		t.Fatal("accepted missing directory")
	}
	if err := os.WriteFile(a.dir, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetDataDirectory(); err == nil {
		t.Fatal("accepted a file as a directory")
	}
}
