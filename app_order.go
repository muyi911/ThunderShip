package main

import (
	"fmt"
	"maps"
	"shell_workflow/internal/core"
	"slices"
	"sort"
)

// MoveResource changes display order only; workflow execution dependencies stay intact.
func (a *App) MoveResource(kind, id, beforeID string) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	c := a.config
	var ids []string
	switch kind {
	case "servers":
		for _, item := range c.Servers {
			ids = append(ids, item.ID)
		}
	case "artifacts":
		for _, item := range c.Artifacts {
			ids = append(ids, item.ID)
		}
	case "workflows":
		for _, item := range c.Workflows {
			ids = append(ids, item.ID)
		}
		for _, item := range c.Combinations {
			ids = append(ids, item.ID)
		}
	default:
		return c, fmt.Errorf("不支持调整此资源的顺序")
	}
	if !slices.Contains(ids, id) || (beforeID != "" && !slices.Contains(ids, beforeID)) {
		return c, fmt.Errorf("资源不存在，请刷新后重试")
	}
	if id == beforeID {
		return c, nil
	}
	rank := map[string]int{}
	for i, item := range c.ResourceOrder[kind] {
		rank[item] = i
	}
	position := func(item string) int {
		if i, ok := rank[item]; ok {
			return i
		}
		return len(c.ResourceOrder[kind])
	}
	sort.SliceStable(ids, func(i, j int) bool { return position(ids[i]) < position(ids[j]) })
	from := slices.Index(ids, id)
	ids = slices.Delete(ids, from, from+1)
	to := len(ids)
	if beforeID != "" {
		to = slices.Index(ids, beforeID)
	}
	ids = slices.Insert(ids, to, id)
	c.ResourceOrder = maps.Clone(c.ResourceOrder)
	if c.ResourceOrder == nil {
		c.ResourceOrder = map[string][]string{}
	}
	c.ResourceOrder[kind] = ids
	return c, a.save(c)
}
