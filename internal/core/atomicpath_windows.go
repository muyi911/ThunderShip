package core

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
)

// MSIX can redirect AppData to another volume; both rename paths must use the same physical directory.
func atomicPath(filename string) (string, error) {
	dir, err := ResolveDirectory(filepath.Dir(filename))
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(filename)), nil
}

// ResolveDirectory returns the physical path, including Windows AppData redirection.
func ResolveDirectory(path string) (string, error) {
	dir, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(windows.Handle(dir.Fd()), &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) >= len(buf) {
			buf = make([]uint16, n+1)
			continue
		}
		return windows.UTF16ToString(buf[:n]), nil
	}
}
