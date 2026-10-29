package core

import (
	"fmt"
	"strings"
)

type ArtifactResource struct {
	ID    string        `json:"id"`
	Name  string        `json:"name"`
	Files []FileMapping `json:"files"`
}

func (c Config) ArtifactResource(id string) (ArtifactResource, bool) {
	for _, asset := range c.Artifacts {
		if asset.ID == id {
			return asset, true
		}
	}
	return ArtifactResource{}, false
}

func ValidateArtifact(asset ArtifactResource) error {
	if strings.TrimSpace(asset.Name) == "" || len(asset.Files) == 0 {
		return fmt.Errorf("请填写构建物名称并添加文件或文件夹")
	}
	seen := map[string]bool{}
	for _, file := range asset.Files {
		if file.ID == "" || seen[file.ID] || (file.Kind != "" && file.Kind != "file" && file.Kind != "folder") {
			return fmt.Errorf("构建物文件 ID 为空、重复或类型无效")
		}
		seen[file.ID] = true
	}
	return nil
}

// References keep local inputs in one place; destinations and executable bits belong to each upload.
func ResolveUploads(w Workflow, c Config) (Workflow, error) {
	w.Steps = append([]Step(nil), w.Steps...)
	for i, st := range w.Steps {
		if st.ArtifactID == "" {
			continue
		}
		if st.Type != "upload" {
			return w, fmt.Errorf("%s: 仅上传节点能引用构建物", st.Name)
		}
		source, ok := c.ArtifactResource(st.ArtifactID)
		if !ok {
			return w, fmt.Errorf("%s: 引用的构建物不存在", st.Name)
		}
		if err := ValidateArtifact(source); err != nil {
			return w, err
		}
		files := map[string]FileMapping{}
		for _, f := range source.Files {
			files[f.ID] = f
		}
		w.Steps[i].Files = append([]FileMapping(nil), st.Files...)
		for j, mapping := range st.Files {
			file, ok := files[mapping.Source]
			if !ok {
				return w, fmt.Errorf("%s: 引用的构建物文件已移除，请重新选择", st.Name)
			}
			w.Steps[i].Files[j].Local = file.Local
			w.Steps[i].Files[j].Kind = file.Kind
		}
	}
	return w, nil
}
