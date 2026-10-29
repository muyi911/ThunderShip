package main

import "shell_workflow/internal/core"

func (a *App) ReadRemoteText(id, remote string, sudo bool) (core.RemoteText, error) {
	if err := remotePath(remote); err != nil {
		return core.RemoteText{}, err
	}
	r, _, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return core.RemoteText{}, err
	}
	defer cancel()
	defer r.Close()
	return r.ReadText(remote)
}

func (a *App) SaveRemoteText(id, remote, content, version string, sudo bool) (core.RemoteText, error) {
	if err := remotePath(remote); err != nil {
		return core.RemoteText{}, err
	}
	c, err := a.getConnection(id)
	if err != nil {
		return core.RemoteText{}, err
	}
	cfg, err := a.snapshot()
	if err != nil {
		return core.RemoteText{}, err
	}
	r, ctx, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return core.RemoteText{}, err
	}
	defer cancel()
	defer r.Close()
	release, err := a.engine.LockTransfer(ctx, cfg, c.server, []core.FileMapping{{Remote: remote}})
	if err != nil {
		return core.RemoteText{}, err
	}
	defer release()
	return r.SaveText(remote, content, version)
}
