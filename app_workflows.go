package main

import (
	"fmt"
	"maps"
	"shell_workflow/internal/core"
	"strings"
)

func (a *App) SaveWorkflow(w core.Workflow) (core.Config, error) {
	w = core.NormalizeWorkflow(w)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	if errs := core.ValidateWorkflow(w, a.config, false); len(errs) > 0 {
		return a.config, fmt.Errorf("%s", strings.Join(errs, "\n"))
	}
	c := a.config
	c.Workflows = append([]core.Workflow(nil), c.Workflows...)
	idx := -1
	for i, old := range c.Workflows {
		if old.ID == w.ID {
			if old.Version != w.Version {
				return a.config, fmt.Errorf("工作流已被修改，请重新加载后合并")
			}
			idx = i
		}
	}
	if w.ID == "" {
		w.ID = core.ID()
	}
	w.Version++
	if idx < 0 {
		c.Workflows = append(c.Workflows, w)
	} else {
		c.Workflows[idx] = w
	}
	return c, a.save(c)
}
func (a *App) SaveCombination(cmb core.Combination) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	if strings.TrimSpace(cmb.Name) == "" {
		return a.config, fmt.Errorf("请填写组合名称")
	}
	c := a.config
	c.Combinations = append([]core.Combination(nil), c.Combinations...)
	if cmb.ID == "" {
		cmb.ID = core.ID()
	}
	found := false
	for i, x := range c.Combinations {
		if x.ID == cmb.ID {
			c.Combinations[i] = cmb
			found = true
		}
	}
	if !found {
		c.Combinations = append(c.Combinations, cmb)
	}
	if e := core.ValidateNode(core.Node{WorkflowID: cmb.ID}, c); e != nil {
		return a.config, e
	}
	return c, a.save(c)
}
func (a *App) Delete(kind, id string) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	for _, run := range a.engine.List() {
		if run.Status == "running" {
			return a.config, fmt.Errorf("请等待当前运行结束后再删除资源")
		}
	}
	c := a.config
	if kind == "workflow" || kind == "combination" {
		for _, w := range c.Workflows {
			if w.ID == id {
				continue
			}
			for _, st := range w.Steps {
				if st.Type == "workflow" && st.WorkflowID == id {
					return c, fmt.Errorf("工作流 %s 仍引用此资源", w.Name)
				}
			}
		}
		for _, cmb := range c.Combinations {
			if cmb.ID == id {
				continue
			}
			for _, ref := range core.References(cmb.Root) {
				if ref == id {
					return c, fmt.Errorf("工作流 %s 仍引用此资源", cmb.Name)
				}
			}
		}
	}
	switch kind {
	case "artifact":
		for _, w := range c.Workflows {
			for _, st := range w.Steps {
				if st.ArtifactID == id {
					return c, fmt.Errorf("工作流 %s 仍引用此构建物", w.Name)
				}
			}
		}
		out := []core.ArtifactResource{}
		for _, asset := range c.Artifacts {
			if asset.ID != id {
				out = append(out, asset)
			}
		}
		c.Artifacts = out
	case "workflow":
		out := []core.Workflow{}
		for _, x := range c.Workflows {
			if x.ID != id {
				out = append(out, x)
			}
		}
		c.Workflows = out
	case "server":
		for _, w := range c.Workflows {
			for _, sid := range core.WorkflowServers(w) {
				if sid == id {
					return c, fmt.Errorf("工作流 %s 仍引用此服务器", w.Name)
				}
			}
		}
		out := []core.Server{}
		for _, x := range c.Servers {
			if x.ID != id {
				out = append(out, x)
			}
		}
		c.Servers = out
		c.Tunnels = maps.Clone(c.Tunnels)
		delete(c.Tunnels, id)
	case "combination":
		out := []core.Combination{}
		for _, x := range c.Combinations {
			if x.ID != id {
				out = append(out, x)
			}
		}
		c.Combinations = out
	case "provider":
		out := []core.Provider{}
		for _, x := range c.Providers {
			if x.ID != id {
				out = append(out, x)
			}
		}
		c.Providers = out
	default:
		return c, fmt.Errorf("不支持删除此类型")
	}
	if err := a.save(c); err != nil {
		return c, err
	}
	if kind == "server" || kind == "provider" {
		if kind == "server" {
			for _, conn := range a.connections {
				if conn.server.ID == id {
					for _, tunnel := range conn.tunnels {
						tunnel.Close()
					}
				}
			}
		}
		if err := core.DeleteSecret(kind + "/" + id); err != nil {
			return c, fmt.Errorf("资源已删除，但凭据清理失败: %w", err)
		}
	}
	return c, nil
}
func (a *App) SaveSettings(concurrency int, theme, uploadProtocol string) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if concurrency < 1 || concurrency > 64 {
		return a.config, fmt.Errorf("并发数须为 1–64")
	}
	if theme != "light" && theme != "dark" && theme != "system" {
		return a.config, fmt.Errorf("主题无效")
	}
	if uploadProtocol != "sftp" && uploadProtocol != "scp" {
		return a.config, fmt.Errorf("上传方式须为 SFTP 或 SCP")
	}
	c := a.config
	c.Concurrency = concurrency
	c.Theme = theme
	c.UploadProtocol = uploadProtocol
	return c, a.save(c)
}
