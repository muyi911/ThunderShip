package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Artifact struct {
	Local      string `json:"local"`
	Remote     string `json:"remote"`
	Executable bool   `json:"executable"`
	Size       int64  `json:"size"`
	Hash       string `json:"hash"`
}

func uploadPath(f FileMapping) string {
	if f.IncludeFolder {
		return path.Join(f.Remote, filepath.Base(f.Local))
	}
	return f.Remote
}

func validUploadPath(f FileMapping) bool {
	return (ValidRemote(f.Remote) || (f.IncludeFolder && f.Remote == "/")) &&
		ValidRemote(uploadPath(f)) && (!f.IncludeFolder || ValidRemote("/"+filepath.Base(f.Local)))
}

func Expand(files []FileMapping) ([]Artifact, error) {
	var result []Artifact
	for _, f := range files {
		if !validUploadPath(f) {
			return nil, fmt.Errorf("无效目标路径: %s", f.Remote)
		}
		info, e := os.Lstat(f.Local)
		if e != nil {
			return nil, e
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("不支持符号链接: %s", f.Local)
		}
		if f.IncludeFolder && !info.IsDir() {
			return nil, fmt.Errorf("只有文件夹能使用“包含文件夹本身”: %s", f.Local)
		}
		remote := uploadPath(f)
		add := func(local, remote string, info fs.FileInfo) error {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("只支持普通文件: %s", local)
			}
			for _, old := range result {
				if Overlap(old.Remote, remote) {
					return fmt.Errorf("目标文件冲突: %s 与 %s", old.Remote, remote)
				}
			}
			result = append(result, Artifact{Local: local, Remote: remote, Executable: f.Executable, Size: info.Size()})
			return nil
		}
		if !info.IsDir() {
			if e = add(f.Local, remote, info); e != nil {
				return nil, e
			}
			continue
		}
		e = filepath.WalkDir(f.Local, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("文件夹包含符号链接: %s", p)
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(f.Local, p)
			if err != nil {
				return err
			}
			i, err := d.Info()
			if err != nil {
				return err
			}
			return add(p, path.Join(remote, filepath.ToSlash(rel)), i)
		})
		if e != nil {
			return nil, e
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("没有可上传的文件（空文件夹不会发布）")
	}
	return result, nil
}
func Freeze(c Config, n Node, dir string) (Config, error) {
	used := map[string]bool{}
	var visit func(Node)
	visit = func(n Node) {
		if n.WorkflowID != "" {
			if used[n.WorkflowID] {
				return
			}
			used[n.WorkflowID] = true
			if w, leaf := c.Workflow(n.WorkflowID); leaf {
				for _, st := range w.Steps {
					if st.Type == "workflow" {
						visit(Node{WorkflowID: st.WorkflowID})
					}
				}
			} else {
				ref, _ := c.Reference(n.WorkflowID)
				visit(ref)
			}
		}
		if n.Graph != nil {
			for _, v := range n.Graph.Nodes {
				if v.Ref == StartRef {
					continue
				}
				visit(Node{WorkflowID: v.Ref})
			}
		}
		for _, x := range n.Children {
			visit(x)
		}
	}
	visit(n)
	cache := map[string]string{}
	freezeFile := func(local, dst string) (string, error) {
		if frozen, ok := cache[local]; ok {
			return frozen, nil
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
			return "", err
		}
		if _, err := copyStable(local, dst); err != nil {
			return "", err
		}
		cache[local], cache[dst] = dst, dst
		return dst, nil
	}
	usedAssets := map[string]map[string]bool{}
	for _, w := range c.Workflows {
		if !used[w.ID] {
			continue
		}
		for _, st := range w.Steps {
			if st.Type != "upload" || st.ArtifactID == "" {
				continue
			}
			if usedAssets[st.ArtifactID] == nil {
				usedAssets[st.ArtifactID] = map[string]bool{}
			}
			for _, file := range st.Files {
				usedAssets[st.ArtifactID][file.Source] = true
			}
		}
	}
	c.Artifacts = append([]ArtifactResource(nil), c.Artifacts...)
	for i, asset := range c.Artifacts {
		if usedAssets[asset.ID] == nil {
			continue
		}
		c.Artifacts[i].Files = append([]FileMapping(nil), asset.Files...)
		for j, mapping := range asset.Files {
			if !usedAssets[asset.ID][mapping.ID] {
				continue
			}
			files, err := Expand([]FileMapping{{Local: mapping.Local, Remote: "/artifact"}})
			if err != nil {
				return c, err
			}
			// Keep the original basename so folder inclusion also works with frozen sources.
			root := filepath.Join(dir, "sources", ID(), filepath.Base(mapping.Local))
			for _, a := range files {
				dst := root + filepath.FromSlash(strings.TrimPrefix(a.Remote, "/artifact"))
				if err = os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
					return c, err
				}
				if _, err = copyStable(a.Local, dst); err != nil {
					return c, err
				}
				cache[dst] = dst
			}
			c.Artifacts[i].Files[j].Local = root
		}
	}
	var workflows []Workflow
	for _, w := range c.Workflows {
		if !used[w.ID] {
			continue
		}
		var err error
		w, err = ResolveUploads(w, c)
		if err != nil {
			return c, err
		}
		for j, s := range w.Steps {
			if s.Type != "upload" {
				continue
			}
			for _, mapping := range s.Files {
				w.Steps[j].LockPaths = append(w.Steps[j].LockPaths, uploadPath(mapping))
			}
			list, e := Expand(s.Files)
			if e != nil {
				return c, e
			}
			var frozen []FileMapping
			for _, a := range list {
				dst := filepath.Join(dir, "sources", ID())
				dst, e = freezeFile(a.Local, dst)
				if e != nil {
					return c, e
				}
				frozen = append(frozen, FileMapping{ID: ID(), Local: dst, Remote: a.Remote, Executable: a.Executable})
			}
			w.Steps[j].Files = frozen
			w.Steps[j].ArtifactID = ""
		}
		workflows = append(workflows, w)
	}
	c.Workflows = workflows
	return c, nil
}
func copyStable(src, dst string) (string, error) {
	f, e := os.Open(src)
	if e != nil {
		return "", e
	}
	defer f.Close()
	before, e := f.Stat()
	if e != nil {
		return "", e
	}
	o, e := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	_, e = io.Copy(io.MultiWriter(o, h), f)
	closeErr := o.Close()
	if e != nil {
		return "", e
	}
	if closeErr != nil {
		return "", closeErr
	}
	after, e := os.Stat(src)
	if e != nil {
		return "", e
	}
	if !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return "", fmt.Errorf("快照期间源文件被修改: %s", src)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func FileHash(filename string) (string, error) {
	f, e := os.Open(filename)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	_, e = io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil)), e
}
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }
