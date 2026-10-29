package main

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"shell_workflow/internal/core"
	"strings"
	"time"
)

func (a *App) SaveProvider(p core.Provider, key string) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	if strings.TrimSpace(p.Name) == "" {
		return a.config, fmt.Errorf("请填写提供方名称")
	}
	if p.Mode != "auto" && p.Mode != "schema" && p.Mode != "json" {
		return a.config, fmt.Errorf("输出模式无效")
	}
	c := a.config
	c.Providers = append([]core.Provider(nil), c.Providers...)
	idx := -1
	for i, old := range c.Providers {
		if old.ID == p.ID {
			idx = i
			p.HasSecret = old.HasSecret
			p.Models = old.Models
			if old.BaseURL != p.BaseURL {
				p.Models = []string{}
				if key == "" && old.HasSecret {
					return c, fmt.Errorf("地址已变化，请重新输入 Key，避免将旧 Key 发给新地址")
				}
			}
		}
	}
	if p.ID == "" {
		p.ID = core.ID()
	}
	if idx < 0 {
		p.HasSecret = false
	}
	if key != "" {
		if e := core.SetSecret("provider/"+p.ID, key); e != nil {
			return c, e
		}
		p.HasSecret = true
	}
	if idx < 0 {
		c.Providers = append(c.Providers, p)
	} else {
		c.Providers[idx] = p
	}
	return c, a.save(c)
}
func (a *App) provider(id string) (core.Provider, string, error) {
	c, e := a.snapshot()
	if e != nil {
		return core.Provider{}, "", e
	}
	for _, p := range c.Providers {
		if p.ID == id {
			key := ""
			if p.HasSecret {
				key, e = core.GetSecret("provider/" + id)
			}
			return p, key, e
		}
	}
	return core.Provider{}, "", fmt.Errorf("请先保存 AI 提供方")
}
func (a *App) ListModels(id string) ([]string, error) {
	p, key, e := a.provider(id)
	if e != nil {
		return nil, e
	}
	models, e := core.ListModels(a.ctx, p, key)
	if e != nil {
		return nil, e
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Providers = append([]core.Provider(nil), c.Providers...)
	for i, x := range c.Providers {
		if x.ID == id && x.BaseURL == p.BaseURL {
			c.Providers[i].Models = models
		}
	}
	return models, a.save(c)
}
func (a *App) TestModel(id string) (string, error) {
	p, key, e := a.provider(id)
	if e != nil {
		return "", e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	content, e := core.Completion(ctx, p, key, "Return JSON only: {\"ok\":true}", "Test connection", map[string]any{"type": "object", "properties": map[string]any{"ok": map[string]any{"type": "boolean"}}, "required": []string{"ok"}, "additionalProperties": false})
	if e != nil {
		return "", e
	}
	var out struct {
		OK bool `json:"ok"`
	}
	if json.Unmarshal([]byte(content), &out) != nil || !out.OK {
		return "", fmt.Errorf("模型未返回预期 JSON")
	}
	return "生成测试成功", nil
}
func (a *App) GenerateDraft(req core.AIRequest, requestID string) (core.Draft, error) {
	p, key, e := a.provider(req.ProviderID)
	if e != nil {
		return core.Draft{}, e
	}
	c, e := a.snapshot()
	if e != nil {
		return core.Draft{}, e
	}
	ctx, cancel := context.WithTimeout(a.ctx, 120*time.Second)
	a.mu.Lock()
	if _, ok := a.ai[requestID]; ok {
		a.mu.Unlock()
		cancel()
		return core.Draft{}, fmt.Errorf("请求仍在进行")
	}
	a.ai[requestID] = cancel
	a.mu.Unlock()
	defer func() { cancel(); a.mu.Lock(); delete(a.ai, requestID); a.mu.Unlock() }()
	return core.Generate(ctx, c, p, key, req)
}
func (a *App) CancelGeneration(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cancel := a.ai[id]; cancel != nil {
		cancel()
	}
}
func (a *App) DraftSchema() map[string]any { return core.Schema(reflect.TypeOf(core.Draft{})) }
