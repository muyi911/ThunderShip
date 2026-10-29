package main

import (
	"context"
	"fmt"
	"shell_workflow/internal/core"
	"time"
)

func (a *App) RunInputs(root core.Node) ([]core.ArtifactInput, error) {
	c, err := a.snapshot()
	if err != nil {
		return nil, err
	}
	return core.RunInputs(c, root)
}
func (a *App) Preflight(root core.Node) ([]string, error) {
	c, err := a.snapshot()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 90*time.Second)
	defer cancel()
	return a.engine.Preflight(ctx, c, root), nil
}
func (a *App) StartRun(root core.Node, name string) (string, error) {
	c, err := a.snapshot()
	if err != nil {
		return "", err
	}
	return a.engine.Start(a.ctx, c, root, name)
}
func (a *App) ListRuns() ([]core.Run, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	return a.engine.List(), nil
}
func (a *App) RunEvents(id string, after int) ([]core.Event, error) {
	if a.initErr != nil {
		return nil, a.initErr
	}
	for _, r := range a.engine.List() {
		if r.ID == id {
			return a.engine.Events(id, after), nil
		}
	}
	return nil, fmt.Errorf("运行不存在")
}
func (a *App) CancelRun(id string) {
	if a.engine != nil {
		a.engine.Cancel(id)
	}
}
