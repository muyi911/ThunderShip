package main

import (
	"context"
	"errors"
	"fmt"
	"shell_workflow/internal/core"
	"strings"
	"time"
)

func (a *App) SaveServer(s core.Server, secret string) (core.Config, error) {
	s.Group = strings.TrimSpace(s.Group)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	if strings.TrimSpace(s.Name) == "" {
		return a.config, fmt.Errorf("请填写服务器名称")
	}
	if err := validateConnection(s); err != nil {
		return a.config, err
	}
	if s.Concurrency < 0 || s.Concurrency > 64 {
		return a.config, fmt.Errorf("同机最大并发应为 0–64，0 表示不限制")
	}
	c := a.config
	c.Servers = append([]core.Server(nil), c.Servers...)
	idx := -1
	clearSecret := false
	for i, old := range c.Servers {
		if old.ID == s.ID {
			idx = i
			s.HasSecret = old.HasSecret && old.Host == s.Host && old.Port == s.Port && old.User == s.User && old.Auth == s.Auth && old.KeyPath == s.KeyPath
			clearSecret = old.HasSecret && !s.HasSecret && secret == ""
			if old.Host == s.Host && old.Port == s.Port && old.Fingerprint != "" {
				s.Fingerprint = old.Fingerprint
			} else if (old.Host != s.Host || old.Port != s.Port) && s.Fingerprint == old.Fingerprint {
				s.Fingerprint = ""
			}
		}
	}
	if s.ID == "" {
		s.ID = core.ID()
	}
	if idx < 0 {
		s.HasSecret = false
	}
	if s.SudoUser == "" {
		s.SudoUser = "root"
	}
	if secret != "" {
		if e := core.SetSecret("server/"+s.ID, secret); e != nil {
			return a.config, e
		}
		s.HasSecret = true
	}
	if idx < 0 {
		c.Servers = append(c.Servers, s)
	} else {
		c.Servers[idx] = s
	}
	if err := a.save(c); err != nil {
		return c, err
	}
	if clearSecret {
		if err := core.DeleteSecret("server/" + s.ID); err != nil {
			return c, fmt.Errorf("配置已保存，旧连接凭据清理失败: %w", err)
		}
	}
	return c, nil
}

func validateConnection(s core.Server) error {
	if strings.TrimSpace(s.Host) == "" || strings.TrimSpace(s.User) == "" || s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("请填写主机、登录用户和有效端口")
	}
	if strings.ContainsAny(s.Host, "\x00\r\n /\\") {
		return fmt.Errorf("主机地址无效")
	}
	if s.Auth != "key" && s.Auth != "password" {
		return fmt.Errorf("请选择密码或私钥认证")
	}
	if s.Fingerprint != "" && !validFingerprint(s.Fingerprint) {
		return fmt.Errorf("指纹无效")
	}
	return nil
}

func validFingerprint(fingerprint string) bool {
	return strings.HasPrefix(fingerprint, "SHA256:") && len(fingerprint) >= 20
}

type ProbeResult struct {
	OK          bool   `json:"ok"`
	Fingerprint string `json:"fingerprint"`
	Changed     bool   `json:"changed"`
	Message     string `json:"message"`
}

func (a *App) ProbeServer(id string) (ProbeResult, error) {
	c, e := a.snapshot()
	if e != nil {
		return ProbeResult{}, e
	}
	s, ok := c.Server(id)
	if !ok {
		return ProbeResult{}, fmt.Errorf("服务器不存在")
	}
	return a.ProbeConnection(s, "")
}

// ProbeConnection tests a draft without saving it or its credentials.
func (a *App) ProbeConnection(s core.Server, secret string) (ProbeResult, error) {
	if err := validateConnection(s); err != nil {
		return ProbeResult{}, err
	}
	c, err := a.snapshot()
	if err != nil {
		return ProbeResult{}, err
	}
	if s.ID != "" {
		old, ok := c.Server(s.ID)
		if !ok {
			return ProbeResult{}, fmt.Errorf("服务器不存在")
		}
		sameEndpoint := s.Host == old.Host && s.Port == old.Port
		if sameEndpoint && old.Fingerprint != "" {
			s.Fingerprint = old.Fingerprint
		} else if !sameEndpoint && s.Fingerprint == old.Fingerprint {
			s.Fingerprint = ""
		}
		if secret == "" && old.HasSecret && sameEndpoint && s.User == old.User && s.Auth == old.Auth && s.KeyPath == old.KeyPath {
			secret, err = core.GetSecret("server/" + old.ID)
			if err != nil {
				return ProbeResult{}, err
			}
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	r, e := core.Dial(ctx, s, secret, true)
	if e != nil {
		var hk *core.HostKeyError
		if errors.As(e, &hk) {
			return ProbeResult{Fingerprint: hk.Fingerprint, Changed: hk.Changed, Message: hk.Error()}, nil
		}
		return ProbeResult{}, e
	}
	r.Close()
	message := "SSH / SFTP 连接成功"
	if s.SudoUpload {
		message = "SSH / sudo SFTP 连接成功"
	}
	return ProbeResult{OK: true, Fingerprint: s.Fingerprint, Message: message}, nil
}
func (a *App) TrustHost(id, fingerprint string) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c := a.config
	c.Servers = append([]core.Server(nil), c.Servers...)
	for i, s := range c.Servers {
		if s.ID == id {
			if s.Fingerprint != "" {
				return c, fmt.Errorf("已有指纹变化不能直接覆盖，请核查后新建服务器记录")
			}
			if !validFingerprint(fingerprint) {
				return c, fmt.Errorf("指纹无效")
			}
			c.Servers[i].Fingerprint = fingerprint
			return c, a.save(c)
		}
	}
	return c, fmt.Errorf("服务器不存在")
}
func (a *App) DetectSFTP(id string) ([]string, error) {
	c, e := a.snapshot()
	if e != nil {
		return nil, e
	}
	s, ok := c.Server(id)
	if !ok {
		return nil, fmt.Errorf("服务器不存在")
	}
	s.SudoUpload = false
	secret := ""
	if s.HasSecret {
		secret, e = core.GetSecret("server/" + id)
		if e != nil {
			return nil, e
		}
	}
	ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
	defer cancel()
	r, e := core.Dial(ctx, s, secret, true)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	var out strings.Builder
	e = r.Run(ctx, core.Step{Command: "for p in /usr/lib/openssh/sftp-server /usr/libexec/openssh/sftp-server /usr/lib/sftp-server /bin/sftp-server; do test -x \"$p\" && printf '%s\\n' \"$p\"; done; true"}, func(s string) { out.WriteString(s) })
	return strings.Fields(out.String()), e
}
