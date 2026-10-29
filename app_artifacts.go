package main

import (
	"fmt"
	"shell_workflow/internal/core"
)

func (a *App) SaveArtifact(asset core.ArtifactResource) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	if err := core.ValidateArtifact(asset); err != nil {
		return a.config, err
	}
	c := a.config
	c.Artifacts = append([]core.ArtifactResource(nil), c.Artifacts...)
	if asset.ID == "" {
		asset.ID = core.ID()
	}
	found := false
	for i, old := range c.Artifacts {
		if old.ID == asset.ID {
			c.Artifacts[i] = asset
			found = true
			break
		}
	}
	if !found {
		c.Artifacts = append(c.Artifacts, asset)
	}
	for _, w := range c.Workflows {
		used := false
		for _, st := range w.Steps {
			if st.ArtifactID == asset.ID {
				used = true
			}
		}
		if _, err := core.ResolveUploads(w, c); used && err != nil {
			return a.config, fmt.Errorf("工作流 %s: %w", w.Name, err)
		}
	}
	return c, a.save(c)
}
