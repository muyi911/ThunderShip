package core

import (
	"fmt"
	"os"
	"path/filepath"
)

type ArtifactInput struct {
	WorkflowID   string `json:"workflowId"`
	ArtifactID   string `json:"artifactId"`
	FileID       string `json:"fileId"`
	WorkflowName string `json:"workflowName"`
	Name         string `json:"name"`
	Kind         string `json:"kind"`
	Local        string `json:"local"`
	Error        string `json:"error"`
}

func RunInputs(c Config, n Node) ([]ArtifactInput, error) {
	ws, err := selected(c, n, false)
	if err != nil {
		return nil, err
	}
	inputs := []ArtifactInput{}
	seen := map[[3]string]bool{}
	for _, w := range ws {
		for _, st := range w.Steps {
			if st.Type != "upload" {
				continue
			}
			wid, aid, label, files := w.ID, st.ID, w.Name, st.Files
			if st.ArtifactID != "" {
				asset, _ := c.ArtifactResource(st.ArtifactID)
				wid, aid, label, files = "", asset.ID, asset.Name, asset.Files
			}
			for _, f := range files {
				if st.ArtifactID != "" {
					used := false
					for _, mapping := range st.Files {
						if mapping.Source == f.ID {
							used = true
						}
					}
					if !used {
						continue
					}
				}
				key := [3]string{wid, aid, f.ID}
				if seen[key] {
					continue
				}
				seen[key] = true
				name := f.Name
				if name == "" {
					name = filepath.Base(f.Local)
				}
				kind := f.Kind
				if kind == "" {
					kind = "file"
					if info, err := os.Stat(f.Local); err == nil && info.IsDir() {
						kind = "folder"
					}
				}
				input := ArtifactInput{WorkflowID: wid, ArtifactID: aid, FileID: f.ID, WorkflowName: label, Name: name, Kind: kind, Local: f.Local}
				if err := validateInput(f.Local, kind); err != nil {
					input.Error = err.Error()
				}
				inputs = append(inputs, input)
			}
		}
	}
	return inputs, nil
}

func validateInput(local, kind string) error {
	if local == "" {
		return fmt.Errorf("请选择本次运行的构建物")
	}
	info, err := os.Lstat(local)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("文件或文件夹不存在")
		}
		return fmt.Errorf("文件不存在或无法访问: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || (kind == "folder" && !info.IsDir()) || (kind != "folder" && !info.Mode().IsRegular()) {
		return fmt.Errorf("文件类型不符或为符号链接")
	}
	return nil
}
