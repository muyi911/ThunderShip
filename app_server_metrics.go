package main

import "shell_workflow/internal/core"

func (a *App) ReadServerMetrics(id string) (core.ServerMetrics, error) {
	c, err := a.getConnection(id)
	if err != nil {
		return core.ServerMetrics{}, err
	}
	return c.terminal.Remote.ReadServerMetrics(a.ctx)
}
