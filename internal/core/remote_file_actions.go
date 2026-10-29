package core

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
)

type RemoteFileOperation struct {
	Action string `json:"action"`
	Source string `json:"source"`
	Target string `json:"target"`
	Mode   string `json:"mode"`
}

func (op RemoteFileOperation) Validate() error {
	validPath := func(p string) bool {
		return path.IsAbs(p) && path.Clean(p) == p && p != "/" && !strings.ContainsAny(p, "\x00\r\n\\")
	}
	if !validPath(op.Source) {
		return fmt.Errorf("请输入有效的远端文件路径，不能操作根目录")
	}
	switch op.Action {
	case "mkdir", "create", "delete":
	case "chmod":
		if len(op.Mode) < 3 || len(op.Mode) > 4 || strings.Trim(op.Mode, "01234567") != "" {
			return fmt.Errorf("权限请输入 3 或 4 位八进制数字，例如 644、755")
		}
	case "rename", "copy", "move", "compress", "extract":
		if !validPath(op.Target) || op.Source == op.Target || strings.HasPrefix(op.Target, op.Source+"/") {
			return fmt.Errorf("目标路径无效，不能覆盖原项目或放入自身目录")
		}
		if op.Action == "rename" && path.Dir(op.Source) != path.Dir(op.Target) {
			return fmt.Errorf("重命名必须保留在原目录中")
		}
		if op.Action == "compress" && !strings.HasSuffix(op.Target, ".tar.gz") {
			return fmt.Errorf("压缩文件名称须以 .tar.gz 结尾")
		}
	default:
		return fmt.Errorf("不支持的文件操作")
	}
	return nil
}

func (r *Remote) fileCommand(ctx context.Context, command, sudoUser string) error {
	if sudoUser != "" {
		command = "sudo -n -u " + Quote(sudoUser) + " -- sh -c " + Quote(command)
	}
	var output strings.Builder
	err := r.Run(ctx, Step{Command: command}, func(text string) {
		if remaining := 16*1024 - output.Len(); remaining > 0 {
			output.WriteString(text[:min(len(text), remaining)])
		}
	})
	if err != nil && strings.TrimSpace(output.String()) != "" {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(output.String()))
	}
	return err
}

func (r *Remote) ManageFile(ctx context.Context, op RemoteFileOperation, sudoUser string) error {
	if err := op.Validate(); err != nil {
		return err
	}
	if err := r.noLinks(path.Dir(op.Source)); err != nil {
		return err
	}
	if op.Action == "mkdir" {
		return r.SFTP.Mkdir(op.Source)
	}
	if op.Action == "create" {
		file, err := r.SFTP.OpenFile(op.Source, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return err
		}
		return file.Close()
	}
	info, err := r.SFTP.Lstat(op.Source)
	if err != nil {
		return err
	}
	switch op.Action {
	case "delete":
		if !info.IsDir() {
			return r.SFTP.Remove(op.Source)
		}
		return r.fileCommand(ctx, "rm -r -- "+Quote(op.Source), sudoUser)
	case "chmod":
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("不能通过符号链接修改目标权限")
		}
		bits, _ := strconv.ParseUint(op.Mode, 8, 12)
		mode := os.FileMode(bits & 0777)
		for bit, flag := range map[uint64]os.FileMode{04000: os.ModeSetuid, 02000: os.ModeSetgid, 01000: os.ModeSticky} {
			if bits&bit != 0 {
				mode |= flag
			}
		}
		return r.SFTP.Chmod(op.Source, mode)
	}
	if err := r.noLinks(path.Dir(op.Target)); err != nil {
		return err
	}
	if _, err := r.SFTP.Lstat(op.Target); err == nil {
		return fmt.Errorf("目标已存在，请使用其他名称；不会覆盖原文件")
	} else if !os.IsNotExist(err) {
		return err
	}
	if op.Action == "compress" || op.Action == "extract" {
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("不支持通过符号链接压缩或解压")
		}
		return r.archive(ctx, op, sudoUser)
	}
	if op.Action != "copy" {
		// SFTP v3 rename refuses an existing target, unlike posix-rename.
		return r.SFTP.Rename(op.Source, op.Target)
	}
	stage := path.Join(path.Dir(op.Target), ".thundership-copy-"+ID())
	if err := r.SFTP.Mkdir(stage); err != nil {
		return err
	}
	defer func() {
		if ctx.Err() == nil {
			_ = r.fileCommand(ctx, "rm -rf -- "+Quote(stage), sudoUser)
		}
	}()
	// Copy on the server, then publish without overwriting an existing target.
	prepared := path.Join(stage, "item")
	if err := r.fileCommand(ctx, "cp -a -- "+Quote(op.Source)+" "+Quote(prepared), sudoUser); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%w；复制临时目录保留在 %s", err, stage)
		}
		return err
	}
	return r.SFTP.Rename(prepared, op.Target)
}
