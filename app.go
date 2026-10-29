package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/wailsapp/wails/v3/pkg/application"
	"path/filepath"
	"shell_workflow/internal/core"
	"sync"
)

type App struct {
	desktop     *application.App
	window      *application.WebviewWindow
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
	config      core.Config
	dir         string
	engine      *core.Engine
	initErr     error
	ai          map[string]context.CancelFunc
	connections map[string]connection
}

func NewApp() *App {
	return &App{config: core.EmptyConfig(), ai: map[string]context.CancelFunc{}, connections: map[string]connection{}}
}
func (a *App) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	a.startup(ctx)
	return nil
}

func (a *App) ServiceShutdown() error {
	a.shutdown(a.ctx)
	return nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx, a.cancel = context.WithCancel(ctx)
	if a.initErr != nil {
		return
	}
	if a.dir == "" {
		a.dir, a.initErr = prepareDataDirectory()
		if a.initErr != nil {
			return
		}
	}
	a.config, a.initErr = core.LoadConfig(a.dir)
	if a.initErr != nil {
		return
	}
	a.engine, a.initErr = core.NewEngine(a.dir, func(ctx context.Context, s core.Server) (core.Session, error) {
		key := ""
		var err error
		if s.HasSecret {
			key, err = core.GetSecret("server/" + s.ID)
			if err != nil {
				return nil, fmt.Errorf("无法读取服务器凭据，请重新保存密码或口令")
			}
		}
		return core.DialSSH(ctx, s, key)
	})
	if a.initErr == nil {
		a.engine.OnEvent = func(ev core.Event) { a.desktop.Event.Emit("run:event", ev) }
	}
}
func (a *App) shutdown(context.Context) {
	a.mu.Lock()
	for _, c := range a.connections {
		for _, tunnel := range c.tunnels {
			tunnel.Close()
		}
		for _, cmd := range c.commands {
			cmd.Close()
		}
		c.terminal.Close()
	}
	a.mu.Unlock()
	if a.cancel != nil {
		a.cancel()
	}
	if a.engine != nil {
		a.engine.Close()
	}
}
func (a *App) snapshot() (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	b, e := json.Marshal(a.config)
	if e != nil {
		return a.config, e
	}
	var c core.Config
	e = json.Unmarshal(b, &c)
	return c, e
}
func (a *App) save(c core.Config) error {
	if a.initErr != nil {
		return a.initErr
	}
	if e := core.WriteJSON(filepath.Join(a.dir, "config.json"), c); e != nil {
		return e
	}
	a.config = c
	return nil
}
func (a *App) GetConfig() (core.Config, error) { return a.snapshot() }
