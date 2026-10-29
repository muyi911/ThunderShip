package main

import (
	"fmt"
	"maps"
	"shell_workflow/internal/core"
	"strings"
)

func (a *App) GetTunnelRules(serverID string) ([]core.TunnelRule, error) {
	c, err := a.snapshot()
	if err != nil {
		return nil, err
	}
	_, ok := c.Server(serverID)
	if !ok {
		return nil, fmt.Errorf("服务器不存在")
	}
	return append([]core.TunnelRule{}, c.Tunnels[serverID]...), nil
}

func (a *App) SaveTunnelRule(serverID string, rule core.TunnelRule) ([]core.TunnelRule, error) {
	rule.Name = strings.TrimSpace(rule.Name)
	rule.BindHost = strings.TrimSpace(rule.BindHost)
	rule.TargetHost = strings.TrimSpace(rule.TargetHost)
	if err := rule.Validate(); err != nil {
		return nil, err
	}
	if rule.ID == "" {
		rule.ID = core.ID()
	}
	return a.changeTunnelRule(serverID, rule, false)
}

func (a *App) DeleteTunnelRule(serverID, ruleID string) ([]core.TunnelRule, error) {
	return a.changeTunnelRule(serverID, core.TunnelRule{ID: ruleID}, true)
}

func (a *App) changeTunnelRule(serverID string, rule core.TunnelRule, remove bool) ([]core.TunnelRule, error) {
	a.mu.Lock()
	c := a.config
	if _, ok := c.Server(serverID); !ok {
		a.mu.Unlock()
		return nil, fmt.Errorf("服务器不存在")
	}
	rules := append([]core.TunnelRule{}, c.Tunnels[serverID]...)
	found := false
	for i := range rules {
		if rules[i].ID == rule.ID {
			found = true
			if remove {
				rules = append(rules[:i], rules[i+1:]...)
			} else {
				rules[i] = rule
			}
			break
		}
	}
	if !found && !remove {
		if len(rules) >= 32 {
			a.mu.Unlock()
			return nil, fmt.Errorf("每台服务器最多保存 32 个隧道")
		}
		rules = append(rules, rule)
	}
	c.Tunnels = maps.Clone(c.Tunnels)
	if c.Tunnels == nil {
		c.Tunnels = map[string][]core.TunnelRule{}
	}
	c.Tunnels[serverID] = rules
	if err := a.save(c); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	var stop []*core.Tunnel
	for _, conn := range a.connections {
		if conn.server.ID == serverID && conn.tunnels[rule.ID] != nil {
			stop = append(stop, conn.tunnels[rule.ID])
			delete(conn.tunnels, rule.ID)
		}
	}
	a.mu.Unlock()
	for _, tunnel := range stop {
		tunnel.Close()
	}
	return rules, nil
}

func (a *App) StartTunnel(connectionID, ruleID string) (core.TunnelStatus, error) {
	c, err := a.getConnection(connectionID)
	if err != nil {
		return core.TunnelStatus{}, err
	}
	rules, err := a.GetTunnelRules(c.server.ID)
	if err != nil {
		return core.TunnelStatus{}, err
	}
	var rule core.TunnelRule
	for _, r := range rules {
		if r.ID == ruleID {
			rule = r
			break
		}
	}
	if rule.ID == "" {
		return core.TunnelStatus{}, fmt.Errorf("隧道配置不存在")
	}
	a.mu.Lock()
	old := c.tunnels[ruleID]
	a.mu.Unlock()
	if old != nil && old.Status().Running {
		return old.Status(), nil
	}
	tunnel, err := c.terminal.Remote.StartTunnel(a.ctx, rule)
	if err != nil {
		return core.TunnelStatus{}, err
	}
	a.mu.Lock()
	current, ok := a.connections[connectionID]
	unchanged := false
	if _, exists := a.config.Server(c.server.ID); exists {
		for _, saved := range a.config.Tunnels[c.server.ID] {
			if saved == rule {
				unchanged = true
				break
			}
		}
	}
	if !ok || !unchanged || (current.tunnels[ruleID] != nil && current.tunnels[ruleID].Status().Running) {
		a.mu.Unlock()
		tunnel.Close()
		return core.TunnelStatus{}, fmt.Errorf("连接或隧道配置已变化，请重试")
	}
	current.tunnels[ruleID] = tunnel
	a.mu.Unlock()
	return tunnel.Status(), nil
}

func (a *App) StopTunnel(connectionID, ruleID string) error {
	a.mu.Lock()
	c, ok := a.connections[connectionID]
	tunnel := c.tunnels[ruleID]
	a.mu.Unlock()
	if !ok {
		return fmt.Errorf("连接已关闭")
	}
	if tunnel != nil {
		tunnel.Close()
	}
	return nil
}

func (a *App) ListTunnels(connectionID string) ([]core.TunnelStatus, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.connections[connectionID]
	if !ok {
		return nil, fmt.Errorf("连接已关闭")
	}
	result := []core.TunnelStatus{}
	for _, tunnel := range c.tunnels {
		result = append(result, tunnel.Status())
	}
	return result, nil
}
