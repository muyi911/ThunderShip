package main

import (
	"context"
	"fmt"
	"github.com/pkg/sftp"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"shell_workflow/internal/core"
)

type connection struct {
	terminal *core.Terminal
	server   core.Server
	secret   string
	commands map[string]*core.TerminalCommand
	tunnels  map[string]*core.Tunnel
}
type ConnectionInfo struct {
	ID        string `json:"id"`
	Directory string `json:"directory"`
}
type RemoteEntry struct {
	Name      string `json:"name"`
	Directory bool   `json:"directory"`
	Size      int64  `json:"size"`
	Mode      string `json:"mode"`
	Modified  string `json:"modified"`
	Link      bool   `json:"link"`
	Owner     string `json:"owner"`
	OwnerID   string `json:"ownerId"`
}

func (a *App) connectionServer(serverID string) (core.Server, string, error) {
	c, err := a.snapshot()
	if err != nil {
		return core.Server{}, "", err
	}
	s, ok := c.Server(serverID)
	if !ok {
		return core.Server{}, "", fmt.Errorf("服务器不存在")
	}
	key := ""
	if s.HasSecret {
		key, err = core.GetSecret("server/" + s.ID)
		if err != nil {
			return core.Server{}, "", fmt.Errorf("无法读取服务器凭据，请重新保存")
		}
	}
	return s, key, nil
}
func (a *App) OpenConnection(serverID string) (ConnectionInfo, error) {
	s, key, err := a.connectionServer(serverID)
	if err != nil {
		return ConnectionInfo{}, err
	}
	s.SudoUpload = false
	r, err := core.Dial(a.ctx, s, key, true)
	if err != nil {
		return ConnectionInfo{}, err
	}
	t, err := core.OpenTerminal(a.ctx, r)
	if err != nil {
		return ConnectionInfo{}, err
	}
	dir, err := r.SFTP.Getwd()
	if err != nil {
		dir = "/"
	}
	id := core.ID()
	a.mu.Lock()
	a.connections[id] = connection{terminal: t, server: s, secret: key, commands: map[string]*core.TerminalCommand{}, tunnels: map[string]*core.Tunnel{}}
	a.mu.Unlock()
	return ConnectionInfo{id, dir}, nil
}
func (a *App) getConnection(id string) (connection, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.connections[id]
	if !ok {
		return c, fmt.Errorf("连接已关闭")
	}
	return c, nil
}
func (a *App) ReadTerminal(id string) (core.TerminalRead, error) {
	c, e := a.getConnection(id)
	if e != nil {
		return core.TerminalRead{}, e
	}
	result := c.terminal.WaitPoll()
	if result.Closed {
		a.CloseConnection(id)
	}
	return result, nil
}
func (a *App) WriteTerminal(id, data string) error {
	c, e := a.getConnection(id)
	if e != nil {
		return e
	}
	if len(data) > 1024*1024 {
		return fmt.Errorf("输入过长")
	}
	return c.terminal.Write(data)
}
func (a *App) ResizeTerminal(id string, cols, rows int) error {
	c, e := a.getConnection(id)
	if e != nil {
		return e
	}
	return c.terminal.Resize(cols, rows)
}
func (a *App) CloseConnection(id string) {
	a.mu.Lock()
	c, ok := a.connections[id]
	var commands []*core.TerminalCommand
	var tunnels []*core.Tunnel
	for _, tunnel := range c.tunnels {
		tunnels = append(tunnels, tunnel)
	}
	for _, cmd := range c.commands {
		commands = append(commands, cmd)
	}
	delete(a.connections, id)
	a.mu.Unlock()
	if ok {
		for _, tunnel := range tunnels {
			tunnel.Close()
		}
		for _, cmd := range commands {
			cmd.Close()
		}
		c.terminal.Close()
	}
}
func (a *App) StartCommand(id, command, directory string) (string, error) {
	if err := remotePath(directory); err != nil {
		return "", err
	}
	c, err := a.getConnection(id)
	if err != nil {
		return "", err
	}
	cmd, err := core.StartTerminalCommand(a.ctx, c.terminal.Remote.SSH, command, directory)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.connections[id]; !ok {
		cmd.Close()
		return "", fmt.Errorf("连接已关闭")
	}
	job := core.ID()
	c.commands[job] = cmd
	return job, nil
}
func (a *App) ReadCommand(id, job string) (core.CommandRead, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	c, ok := a.connections[id]
	if !ok || c.commands[job] == nil {
		return core.CommandRead{}, fmt.Errorf("命令或连接已结束")
	}
	out := c.commands[job].Poll()
	if out.Closed {
		delete(c.commands, job)
	}
	return out, nil
}
func (a *App) CancelCommand(id, job string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if cmd := a.connections[id].commands[job]; cmd != nil {
		cmd.Close()
	}
}
func (a *App) WriteCommand(id, job, data string) error {
	a.mu.Lock()
	c, ok := a.connections[id]
	var cmd *core.TerminalCommand
	if ok {
		cmd = c.commands[job]
	}
	a.mu.Unlock()
	if cmd == nil {
		return fmt.Errorf("命令或连接已结束")
	}
	return cmd.Input(data)
}
func (a *App) ResizeCommand(id, job string, cols, rows int) error {
	a.mu.Lock()
	c, ok := a.connections[id]
	var cmd *core.TerminalCommand
	if ok {
		cmd = c.commands[job]
	}
	a.mu.Unlock()
	if cmd == nil {
		return fmt.Errorf("命令或连接已结束")
	}
	return cmd.Resize(cols, rows)
}
func (a *App) fileRemote(id string, sudo bool) (*core.Remote, context.Context, context.CancelFunc, error) {
	c, err := a.getConnection(id)
	if err != nil {
		return nil, nil, nil, err
	}
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Hour)
	s := c.server
	s.SudoUpload = sudo
	r, err := core.Dial(ctx, s, c.secret, true)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return r, ctx, cancel, nil
}
func remotePath(p string) error {
	if !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n\\") {
		return fmt.Errorf("请输入有效的远端绝对路径")
	}
	return nil
}
func (a *App) ListRemote(id, dir string, sudo bool) ([]RemoteEntry, error) {
	if err := remotePath(dir); err != nil {
		return nil, err
	}
	r, ctx, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer r.Close()
	entries, err := r.SFTP.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := r.OwnerNames(ctx, entries)
	out := []RemoteEntry{}
	for _, e := range entries {
		owner := ""
		if stat, ok := e.Sys().(*sftp.FileStat); ok {
			owner = fmt.Sprintf("%d/%d", stat.UID, stat.GID)
		}
		label := names[owner]
		if label == "" {
			label = owner
		}
		out = append(out, RemoteEntry{Name: e.Name(), Directory: e.IsDir(), Size: e.Size(), Mode: e.Mode().String(), Modified: e.ModTime().Format(time.RFC3339), Link: e.Mode()&os.ModeSymlink != 0, Owner: label, OwnerID: owner})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Directory != out[j].Directory {
			return out[i].Directory
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
func (a *App) UploadRemote(id, dir string, sudo, folder, executable bool) error {
	if err := remotePath(dir); err != nil {
		return err
	}
	mappings, err := a.PickFiles(folder)
	if err != nil {
		return err
	}
	return a.uploadRemote(id, dir, mappings, sudo, executable)
}

func (a *App) UploadRemotePaths(id, dir string, paths []string, sudo, executable bool) error {
	if err := remotePath(dir); err != nil {
		return err
	}
	mappings, err := a.ResolveLocalPaths(paths)
	if err != nil {
		return err
	}
	return a.uploadRemote(id, dir, mappings, sudo, executable)
}

func (a *App) uploadRemote(id, dir string, mappings []core.FileMapping, sudo, executable bool) error {
	if len(mappings) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for i := range mappings {
		mappings[i].Remote = path.Join(dir, filepath.Base(mappings[i].Local))
		if seen[mappings[i].Remote] {
			return fmt.Errorf("存在同名上传目标：%s，请分开上传", mappings[i].Remote)
		}
		seen[mappings[i].Remote] = true
		mappings[i].Executable = executable
	}
	return a.uploadMappings(id, mappings, sudo, "")
}

func (a *App) UploadRemoteSelection(id string, mappings []core.FileMapping, sudo bool, protocol string) error {
	if protocol != "" && protocol != "scp" && protocol != "sftp" {
		return fmt.Errorf("上传方式须为 SFTP 或 SCP")
	}
	return a.uploadMappings(id, mappings, sudo, protocol)
}

func (a *App) uploadMappings(id string, mappings []core.FileMapping, sudo bool, protocol string) error {
	if len(mappings) == 0 {
		return nil
	}
	if _, err := core.Expand(mappings); err != nil {
		return err
	}
	// Freeze local sources so a concurrent local rebuild cannot change the transfer.
	tmp, err := os.MkdirTemp("", "shell-workflow-upload-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	cfg := core.EmptyConfig()
	cfg.Workflows = []core.Workflow{{ID: "upload", Steps: []core.Step{{ID: "upload", Type: "upload", Files: mappings}}}}
	frozen, err := core.Freeze(cfg, core.Node{WorkflowID: "upload"}, tmp)
	if err != nil {
		return err
	}
	files, err := core.Expand(frozen.Workflows[0].Steps[0].Files)
	if err != nil {
		return err
	}
	r, ctx, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return err
	}
	defer cancel()
	defer r.Close()
	conn, err := a.getConnection(id)
	if err != nil {
		return err
	}
	cfg, err = a.snapshot()
	if err != nil {
		return err
	}
	release, err := a.engine.LockTransfer(ctx, cfg, conn.server, mappings)
	if err != nil {
		return err
	}
	defer release()
	if protocol == "" {
		protocol = cfg.UploadProtocol
	}
	return r.Publish(ctx, files, core.Step{UploadProtocol: protocol}, func(string, int64, int64, string) {})
}
func (a *App) DownloadRemote(id, remote string, sudo bool) error {
	return a.DownloadRemoteWithProtocol(id, remote, sudo, "sftp")
}

func (a *App) DownloadRemoteWithProtocol(id, remote string, sudo bool, protocol string) error {
	if err := remotePath(remote); err != nil {
		return err
	}
	if protocol != "sftp" && protocol != "scp" {
		return fmt.Errorf("下载方式须为 SFTP 或 SCP")
	}
	r, ctx, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return err
	}
	defer cancel()
	defer r.Close()
	info, err := r.SFTP.Lstat(remote)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() && !info.IsDir() {
		return fmt.Errorf("下载仅支持普通文件")
	}
	filename := path.Base(remote)
	if info.IsDir() {
		filename += ".tar.gz"
	}
	dst, err := a.desktop.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{Title: "下载文件", Filename: filename, CanCreateDirectories: true, Window: a.window}).PromptForSingleSelection()
	if err = dialogError(err); err != nil || dst == "" {
		return err
	}
	if info.IsDir() {
		stage := "/tmp/.thundership-download-" + core.ID()
		if err := r.SFTP.Mkdir(stage); err != nil {
			return err
		}
		user := ""
		if sudo {
			connection, err := a.getConnection(id)
			if err != nil {
				return err
			}
			user = connection.server.SudoUser
			if user == "" {
				user = "root"
			}
		}
		defer func() {
			if ctx.Err() == nil {
				command := "rm -rf -- " + core.Quote(stage)
				if user != "" {
					command = "sudo -n -u " + core.Quote(user) + " -- sh -c " + core.Quote(command)
				}
				_ = r.Run(ctx, core.Step{Command: command}, nil)
			}
		}()
		if err := r.SFTP.Chmod(stage, 0700); err != nil {
			return err
		}
		target := path.Join(stage, "download.tar.gz")
		if err := r.ManageFile(ctx, core.RemoteFileOperation{Action: "compress", Source: remote, Target: target}, user); err != nil {
			return err
		}
		remote = target
	}
	return core.WriteAtomic(dst, func(out *os.File) error { return r.Download(ctx, remote, out, protocol) })
}
