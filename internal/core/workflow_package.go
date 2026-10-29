package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"strings"
)

type WorkflowResource struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type WorkflowPackage struct {
	Format       string             `json:"format"`
	Version      int                `json:"version"`
	Root         string             `json:"root"`
	Workflows    []Workflow         `json:"workflows"`
	Combinations []Combination      `json:"combinations"`
	Servers      []WorkflowResource `json:"servers"`
	Artifacts    []ArtifactResource `json:"artifacts"`
}

type ArtifactBinding struct {
	ID    string            `json:"id"`
	Files map[string]string `json:"files"`
}

type WorkflowBindings struct {
	Servers   map[string]string          `json:"servers"`
	Artifacts map[string]ArtifactBinding `json:"artifacts"`
}

func copyPackage(p WorkflowPackage) WorkflowPackage {
	b, _ := json.Marshal(p)
	var out WorkflowPackage
	_ = json.Unmarshal(b, &out)
	return out
}

func ExportWorkflowPackage(c Config, root string) (WorkflowPackage, error) {
	p := WorkflowPackage{Format: "thundership-workflow", Version: 1, Root: root,
		Workflows: []Workflow{}, Combinations: []Combination{}, Servers: []WorkflowResource{}, Artifacts: []ArtifactResource{}}
	if err := ValidateNode(Node{WorkflowID: root}, c); err != nil {
		return p, err
	}
	used := map[string]bool{}
	var visit func(string)
	visit = func(id string) {
		if used[id] {
			return
		}
		used[id] = true
		if w, ok := c.Workflow(id); ok {
			p.Workflows = append(p.Workflows, NormalizeWorkflow(w))
			for _, st := range w.Steps {
				if st.Type == "workflow" {
					visit(st.WorkflowID)
				}
			}
		} else {
			for _, cmb := range c.Combinations {
				if cmb.ID == id {
					p.Combinations = append(p.Combinations, cmb)
					for _, ref := range References(cmb.Root) {
						visit(ref)
					}
				}
			}
		}
	}
	visit(root)
	p = copyPackage(p)
	servers := map[string]bool{}
	assets := map[string]int{}
	for wi := range p.Workflows {
		w := &p.Workflows[wi]
		w.Version = 0
		for si := range w.Steps {
			st := &w.Steps[si]
			if st.Type == "server" && st.Targets != nil {
				for _, id := range st.Targets.Servers {
					if servers[id] {
						continue
					}
					s, ok := c.Server(id)
					if !ok {
						return p, fmt.Errorf("服务器不存在：%s", id)
					}
					servers[id] = true
					p.Servers = append(p.Servers, WorkflowResource{ID: id, Name: s.Name})
				}
			}
			if st.Type != "upload" {
				continue
			}
			asset, ok := c.ArtifactResource(st.ArtifactID)
			if st.ArtifactID == "" {
				asset = ArtifactResource{ID: ID(), Name: w.Name + " · " + st.Name}
				for fi := range st.Files {
					file := &st.Files[fi]
					file.Source = ID()
					source := *file
					source.ID = file.Source
					asset.Files = append(asset.Files, source)
				}
				st.ArtifactID, ok = asset.ID, true
			}
			if !ok {
				return p, fmt.Errorf("%s：构建物不存在", st.Name)
			}
			idx, exists := assets[asset.ID]
			if !exists {
				idx = len(p.Artifacts)
				assets[asset.ID] = idx
				p.Artifacts = append(p.Artifacts, ArtifactResource{ID: asset.ID, Name: asset.Name, Files: []FileMapping{}})
			}
			for fi := range st.Files {
				file := &st.Files[fi]
				var source FileMapping
				for _, f := range asset.Files {
					if f.ID == file.Source {
						source = f
					}
				}
				if source.ID == "" {
					return p, fmt.Errorf("%s：构建物文件不存在", st.Name)
				}
				if source.Name == "" {
					source.Name = path.Base(strings.ReplaceAll(source.Local, `\`, "/"))
				}
				if source.Kind == "" {
					info, err := os.Stat(source.Local)
					if err != nil {
						return p, fmt.Errorf("%s：无法确定文件或文件夹类型，请先完善构建物配置", source.Name)
					}
					source.Kind = "file"
					if info.IsDir() {
						source.Kind = "folder"
					}
				}
				found := false
				for _, f := range p.Artifacts[idx].Files {
					found = found || f.ID == source.ID
				}
				if !found {
					p.Artifacts[idx].Files = append(p.Artifacts[idx].Files, FileMapping{ID: source.ID, Name: source.Name, Kind: source.Kind})
				}
				file.Local = ""
			}
		}
	}
	return p, ValidateWorkflowPackage(p)
}

func ValidateWorkflowPackage(p WorkflowPackage) error {
	if p.Format != "thundership-workflow" || p.Version != 1 {
		return fmt.Errorf("不是受支持的霆舟工作流文件（需要版本 1）")
	}
	if p.Workflows == nil || p.Combinations == nil || p.Servers == nil || p.Artifacts == nil {
		return fmt.Errorf("工作流文件不完整，请使用霆舟导出的工作流文件")
	}
	if len(p.Workflows)+len(p.Combinations) == 0 || len(p.Workflows)+len(p.Combinations) > 256 || len(p.Servers) > 1024 || len(p.Artifacts) > 1024 {
		return fmt.Errorf("导入文件中的工作流或资源数量无效")
	}
	p = copyPackage(p)
	c := EmptyConfig()
	for _, s := range p.Servers {
		if _, ok := c.Server(s.ID); ok || s.ID == "" || strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("导入文件的服务器标识为空、重复或缺少名称")
		}
		c.Servers = append(c.Servers, Server{ID: s.ID, Name: s.Name, Host: s.ID, Port: 22})
	}
	for _, asset := range p.Artifacts {
		if _, ok := c.ArtifactResource(asset.ID); ok || asset.ID == "" || len(asset.Files) > 4096 {
			return fmt.Errorf("导入文件的构建物标识为空、重复或文件数量过多")
		}
		if err := ValidateArtifact(asset); err != nil {
			return err
		}
		for i, file := range asset.Files {
			if file.Local != "" || file.Name == "" || (file.Kind != "file" && file.Kind != "folder") {
				return fmt.Errorf("导入文件只应包含构建物名称和类型，不能包含本地路径")
			}
			asset.Files[i].Local = "/import-placeholder/" + file.Name
		}
		c.Artifacts = append(c.Artifacts, asset)
	}
	ids := map[string]bool{}
	for _, w := range p.Workflows {
		if w.ID == "" || w.ID == StartRef || ids[w.ID] {
			return fmt.Errorf("导入文件的工作流标识为空或重复")
		}
		ids[w.ID] = true
		for _, st := range w.Steps {
			if st.Type == "upload" && st.ArtifactID == "" {
				return fmt.Errorf("导入的上传节点必须重新关联构建物")
			}
			for _, file := range st.Files {
				if file.Local != "" {
					return fmt.Errorf("工作流文件不能包含本地文件路径")
				}
			}
		}
		c.Workflows = append(c.Workflows, w)
	}
	for _, cmb := range p.Combinations {
		if cmb.ID == "" || cmb.ID == StartRef || ids[cmb.ID] || strings.TrimSpace(cmb.Name) == "" {
			return fmt.Errorf("导入文件的编排标识为空、重复或缺少名称")
		}
		ids[cmb.ID] = true
		c.Combinations = append(c.Combinations, cmb)
	}
	if err := ValidateNode(Node{WorkflowID: p.Root}, c); err != nil {
		return err
	}
	for _, w := range c.Workflows {
		if errs := ValidateWorkflow(w, c, false); len(errs) > 0 {
			return fmt.Errorf("%s：%s", w.Name, strings.Join(errs, "；"))
		}
	}
	for _, cmb := range c.Combinations {
		if err := ValidateNode(Node{WorkflowID: cmb.ID}, c); err != nil {
			return err
		}
	}
	return nil
}

func ImportWorkflowPackage(c Config, p WorkflowPackage, bindings WorkflowBindings) (Config, error) {
	if err := ValidateWorkflowPackage(p); err != nil {
		return c, err
	}
	p = copyPackage(p)
	for _, s := range p.Servers {
		if _, ok := c.Server(bindings.Servers[s.ID]); !ok {
			return c, fmt.Errorf("请重新选择服务器：%s", s.Name)
		}
	}
	for _, asset := range p.Artifacts {
		binding := bindings.Artifacts[asset.ID]
		local, ok := c.ArtifactResource(binding.ID)
		if !ok {
			return c, fmt.Errorf("请重新选择构建物：%s", asset.Name)
		}
		for _, required := range asset.Files {
			found := false
			for _, file := range local.Files {
				if file.ID == binding.Files[required.ID] && file.Kind == required.Kind {
					found = true
				}
			}
			if !found {
				return c, fmt.Errorf("%s：请为 %s 选择对应的%s", asset.Name, required.Name, map[string]string{"file": "文件", "folder": "文件夹"}[required.Kind])
			}
		}
	}
	ids, names := map[string]string{}, map[string]bool{}
	for _, w := range c.Workflows {
		names[w.Name] = true
	}
	for _, cmb := range c.Combinations {
		names[cmb.Name] = true
	}
	newIdentity := func(id, name string) (string, string) {
		ids[id] = ID()
		base := name
		for i := 1; names[name]; i++ {
			name = fmt.Sprintf("%s - 导入 %d", base, i)
		}
		names[name] = true
		return ids[id], name
	}
	for i := range p.Workflows {
		p.Workflows[i] = NormalizeWorkflow(p.Workflows[i])
		p.Workflows[i].ID, p.Workflows[i].Name = newIdentity(p.Workflows[i].ID, p.Workflows[i].Name)
		p.Workflows[i].Version = 1
	}
	for i := range p.Combinations {
		p.Combinations[i].ID, p.Combinations[i].Name = newIdentity(p.Combinations[i].ID, p.Combinations[i].Name)
	}
	var rebindNode func(*Node)
	rebindNode = func(n *Node) {
		if n.WorkflowID != "" {
			n.WorkflowID = ids[n.WorkflowID]
		}
		if n.Graph != nil {
			for i := range n.Graph.Nodes {
				if n.Graph.Nodes[i].Ref != StartRef {
					n.Graph.Nodes[i].Ref = ids[n.Graph.Nodes[i].Ref]
				}
			}
		}
		for i := range n.Children {
			rebindNode(&n.Children[i])
		}
	}
	for wi := range p.Workflows {
		for si := range p.Workflows[wi].Steps {
			st := &p.Workflows[wi].Steps[si]
			if st.Type == "workflow" {
				st.WorkflowID = ids[st.WorkflowID]
			}
			if st.Targets != nil {
				for i, sid := range st.Targets.Servers {
					st.Targets.Servers[i] = bindings.Servers[sid]
				}
			}
			if st.ArtifactID != "" {
				binding := bindings.Artifacts[st.ArtifactID]
				st.ArtifactID = binding.ID
				for i := range st.Files {
					st.Files[i].Source = binding.Files[st.Files[i].Source]
				}
			}
		}
	}
	for i := range p.Combinations {
		rebindNode(&p.Combinations[i].Root)
	}
	out := c
	out.Workflows = append(append([]Workflow(nil), c.Workflows...), p.Workflows...)
	out.Combinations = append(append([]Combination(nil), c.Combinations...), p.Combinations...)
	for _, w := range p.Workflows {
		if errs := ValidateWorkflow(w, out, false); len(errs) > 0 {
			return c, fmt.Errorf("%s：%s", w.Name, strings.Join(errs, "；"))
		}
	}
	for _, cmb := range p.Combinations {
		if err := ValidateNode(Node{WorkflowID: cmb.ID}, out); err != nil {
			return c, err
		}
	}
	return out, nil
}
