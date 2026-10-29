//go:build !windows

package core

import "path/filepath"

func atomicPath(filename string) (string, error) { return filename, nil }

func ResolveDirectory(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
