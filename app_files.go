package main

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"shell_workflow/internal/core"
	"sort"
	"strings"
)

func (a *App) PickFiles(folder bool) ([]core.FileMapping, error) {
	var names []string
	var e error
	if folder {
		var name string
		name, e = a.desktop.Dialog.OpenFile().SetTitle("选择部署文件夹").CanChooseFiles(false).CanChooseDirectories(true).AttachToWindow(a.window).PromptForSingleSelection()
		if name != "" {
			names = []string{name}
		}
	} else {
		names, e = a.desktop.Dialog.OpenFile().SetTitle("添加部署文件").AttachToWindow(a.window).PromptForMultipleSelection()
	}
	if e = dialogError(e); e != nil {
		return nil, e
	}
	return a.ResolveLocalPaths(names)
}

type LocalEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Directory  bool   `json:"directory"`
	Selectable bool   `json:"selectable"`
}

type LocalDirectory struct {
	Path    string       `json:"path"`
	Parent  string       `json:"parent"`
	Entries []LocalEntry `json:"entries"`
}

func (a *App) ListLocalDirectory(dir string) (LocalDirectory, error) {
	var out LocalDirectory
	if dir == "" {
		var err error
		dir, err = os.UserHomeDir()
		if err != nil {
			return out, err
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return out, err
	}
	if info, err := os.Stat(dir); err == nil && !info.IsDir() {
		dir = filepath.Dir(dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out, fmt.Errorf("无法打开目录 %s：%w", dir, err)
	}
	out = LocalDirectory{Path: dir, Parent: filepath.Dir(dir), Entries: []LocalEntry{}}
	for _, entry := range entries {
		out.Entries = append(out.Entries, LocalEntry{
			Name: entry.Name(), Path: filepath.Join(dir, entry.Name()), Directory: entry.IsDir(),
			Selectable: entry.Type().IsRegular() || entry.IsDir(),
		})
	}
	sort.SliceStable(out.Entries, func(i, j int) bool {
		a, b := out.Entries[i], out.Entries[j]
		if a.Directory != b.Directory {
			return a.Directory
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	return out, nil
}

// Resolve again on confirmation so deleted or replaced entries cannot be silently added.
func (a *App) ResolveLocalPaths(paths []string) ([]core.FileMapping, error) {
	out := []core.FileMapping{}
	seen := map[string]bool{}
	for _, name := range paths {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("本地路径不能为空")
		}
		local, err := filepath.Abs(name)
		if err != nil {
			return nil, err
		}
		key := local
		if goruntime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		info, err := os.Lstat(local)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("文件或文件夹不存在：%s", local)
			}
			return nil, fmt.Errorf("无法选择 %s：%w", local, err)
		}
		kind := "file"
		if info.IsDir() {
			kind = "folder"
		} else if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("只支持普通文件或文件夹：%s", local)
		}
		out = append(out, core.FileMapping{ID: core.ID(), Name: filepath.Base(local), Kind: kind, Local: local, Remote: "/opt/app/" + filepath.Base(local)})
	}
	return out, nil
}
func (a *App) PickKey() (string, error) {
	name, err := a.desktop.Dialog.OpenFile().SetTitle("选择 SSH 私钥").AttachToWindow(a.window).PromptForSingleSelection()
	return name, dialogError(err)
}
func (a *App) PreviewFiles(files []core.FileMapping) ([]core.Artifact, error) {
	return core.Expand(files)
}

// Wails 3's Windows cancellation sentinel is internal; keep cancellation a no-op as in v2.
func dialogError(err error) error {
	if err != nil && err.Error() == "cancelled by user" {
		return nil
	}
	return err
}
