package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"shell_workflow/internal/core"
	"time"
)

type UploadFileInfo struct {
	Size     int64  `json:"size"`
	Mode     string `json:"mode"`
	Modified string `json:"modified"`
}

type UploadPreview struct {
	File   core.FileMapping `json:"file"`
	Local  UploadFileInfo   `json:"local"`
	Remote *UploadFileInfo  `json:"remote"`
}

func uploadInfo(info os.FileInfo) UploadFileInfo {
	return UploadFileInfo{Size: info.Size(), Mode: info.Mode().String(), Modified: info.ModTime().Format(time.RFC3339)}
}

func (a *App) PreviewRemoteUpload(id, directory string, paths []string, sudo bool) ([]UploadPreview, error) {
	if err := remotePath(directory); err != nil {
		return nil, err
	}
	mappings, err := a.ResolveLocalPaths(paths)
	if err != nil {
		return nil, err
	}
	for i := range mappings {
		mappings[i].Remote = path.Join(directory, filepath.Base(mappings[i].Local))
	}
	files, err := core.Expand(mappings)
	if err != nil {
		return nil, err
	}
	r, _, cancel, err := a.fileRemote(id, sudo)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer r.Close()
	result := []UploadPreview{}
	for _, file := range files {
		local, err := os.Stat(file.Local)
		if err != nil {
			return nil, err
		}
		item := UploadPreview{File: core.FileMapping{ID: core.ID(), Local: file.Local, Remote: file.Remote}, Local: uploadInfo(local)}
		remote, err := r.SFTP.Lstat(file.Remote)
		if err == nil {
			if !remote.Mode().IsRegular() {
				return nil, fmt.Errorf("目标不是普通文件，不能覆盖：%s", file.Remote)
			}
			info := uploadInfo(remote)
			item.Remote = &info
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		result = append(result, item)
	}
	return result, nil
}
