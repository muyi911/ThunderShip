package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"shell_workflow/internal/core"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func (a *App) ExportWorkflow(id string) (bool, error) {
	c, err := a.snapshot()
	if err != nil {
		return false, err
	}
	p, err := core.ExportWorkflowPackage(c, id)
	if err != nil {
		return false, err
	}
	filename, err := a.desktop.Dialog.SaveFileWithOptions(&application.SaveFileDialogOptions{
		Title: "导出工作流", Filename: "工作流.thundership.json",
		Filters: []application.FileFilter{{DisplayName: "霆舟工作流 (*.json)", Pattern: "*.json"}},
		Window:  a.window, CanCreateDirectories: true,
	}).PromptForSingleSelection()
	if err = dialogError(err); err != nil || filename == "" {
		return false, err
	}
	if err := core.WriteJSON(filename, p); err != nil {
		return false, err
	}
	return true, nil
}

func readWorkflowPackage(filename string) (*core.WorkflowPackage, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const maxSize = 8 << 20
	b, err := io.ReadAll(io.LimitReader(f, maxSize+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxSize {
		return nil, fmt.Errorf("工作流文件不能超过 8 MB")
	}
	var p core.WorkflowPackage
	if err := json.Unmarshal(bytes.TrimPrefix(b, []byte{0xef, 0xbb, 0xbf}), &p); err != nil {
		return nil, fmt.Errorf("无法读取工作流文件：%w", err)
	}
	if err := core.ValidateWorkflowPackage(p); err != nil {
		return nil, err
	}
	return &p, nil
}

func (a *App) ReadWorkflowImport() (*core.WorkflowPackage, error) {
	filename, err := a.desktop.Dialog.OpenFileWithOptions(&application.OpenFileDialogOptions{
		CanChooseFiles: true, Title: "导入工作流", Filters: []application.FileFilter{{DisplayName: "霆舟工作流 (*.json)", Pattern: "*.json"}},
		Window: a.window, CanCreateDirectories: true,
	}).PromptForSingleSelection()
	if err = dialogError(err); err != nil || filename == "" {
		return nil, err
	}
	return readWorkflowPackage(filename)
}

func (a *App) ImportWorkflows(p core.WorkflowPackage, bindings core.WorkflowBindings) (core.Config, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.initErr != nil {
		return a.config, a.initErr
	}
	c, err := core.ImportWorkflowPackage(a.config, p, bindings)
	if err != nil {
		return a.config, err
	}
	if err := a.save(c); err != nil {
		return a.config, err
	}
	return c, nil
}
