package main

import "shell_workflow/internal/core"

func (a *App) ManageRemoteFile(id string, op core.RemoteFileOperation, sudo bool) error {
	if err := op.Validate(); err != nil {
		return err
	}
	c, err := a.getConnection(id)
	if err != nil {
		return err
	}
	cfg, err := a.snapshot()
	if err != nil {
		return err
	}
	r, ctx, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return err
	}
	defer cancel()
	defer r.Close()
	paths := []core.FileMapping{{Remote: op.Source}}
	if op.Target != "" {
		paths = append(paths, core.FileMapping{Remote: op.Target})
	}
	release, err := a.engine.LockTransfer(ctx, cfg, c.server, paths)
	if err != nil {
		return err
	}
	defer release()
	sudoUser := ""
	if sudo {
		sudoUser = c.server.SudoUser
		if sudoUser == "" {
			sudoUser = "root"
		}
	}
	return r.ManageFile(ctx, op, sudoUser)
}
