package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"shell_workflow/internal/core"
)

func prepareDataDirectory() (string, error) {
	if dir := os.Getenv("SHELL_WORKFLOW_DATA"); dir != "" {
		return dir, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return migrateDataDirectory(base)
}

func migrateDataDirectory(base string) (string, error) {
	dir := filepath.Join(base, "ThunderShip")
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
		return dir, nil // Once migrated, the new directory owns all subsequent changes.
	} else if !errors.Is(err, os.ErrNotExist) {
		return dir, err
	}
	old := filepath.Join(base, "ShellWorkflow")
	if _, err := os.Stat(filepath.Join(old, "config.json")); errors.Is(err, os.ErrNotExist) {
		return dir, os.MkdirAll(dir, 0700)
	} else if err != nil {
		return dir, err
	}
	if _, err := core.LoadConfig(old); err != nil {
		return dir, fmt.Errorf("旧配置无法读取，已保留原数据 %s: %w", old, err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return dir, err
	}
	if len(entries) != 0 {
		return dir, fmt.Errorf("数据迁移未执行：%s 已有文件但缺少 config.json；旧数据仍在 %s", dir, old)
	}
	// Resolve the parent before renaming: packaged Windows hosts can redirect AppData across drives.
	physical, err := resolveDataPath(base)
	if err != nil {
		return dir, err
	}
	stage, err := os.MkdirTemp(physical, ".thundership-migrate-*")
	if err != nil {
		return dir, err
	}
	defer os.RemoveAll(stage)
	if err = os.CopyFS(stage, os.DirFS(old)); err != nil {
		return dir, fmt.Errorf("复制旧数据失败，原目录保持完整: %w", err)
	}
	if _, err = core.LoadConfig(stage); err != nil {
		return dir, fmt.Errorf("校验迁移配置失败: %w", err)
	}
	target := filepath.Join(physical, "ThunderShip")
	if err = os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return dir, err // Remove only an empty destination, never overwrite existing data.
	}
	if err = os.Rename(stage, target); err != nil {
		return dir, fmt.Errorf("启用迁移数据失败: %w", err)
	}
	return dir, nil
}
