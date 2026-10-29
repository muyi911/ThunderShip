package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"shell_workflow/internal/core"
	"strings"
)

func (a *App) GetDataDirectory() (string, error) {
	if a.dir == "" {
		return "", fmt.Errorf("数据目录尚未初始化")
	}
	info, err := os.Stat(a.dir)
	if err != nil {
		return "", fmt.Errorf("无法访问数据目录：%w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("数据路径不是文件夹：%s", a.dir)
	}
	dir, err := resolveDataPath(a.dir)
	if err != nil {
		return "", fmt.Errorf("无法解析数据目录：%w", err)
	}
	return dir, nil
}

func resolveDataPath(path string) (string, error) {
	dir, err := core.ResolveDirectory(path)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(dir, `\\?\UNC\`) {
		dir = `\\` + strings.TrimPrefix(dir, `\\?\UNC\`)
	} else {
		dir = strings.TrimPrefix(dir, `\\?\`)
	}
	return dir, nil
}

func (a *App) OpenDataDirectory() error {
	dir, err := a.GetDataDirectory()
	if err != nil {
		return err
	}
	program := "xdg-open"
	switch runtime.GOOS {
	case "windows":
		program = "explorer.exe"
	case "darwin":
		program = "open"
	}
	cmd := exec.Command(program, dir)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("无法打开数据目录：%w", err)
	}
	go cmd.Wait()
	return nil
}
