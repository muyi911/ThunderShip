package main

import (
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var clipboardUser = windows.NewLazySystemDLL("user32.dll")
var clipboardShell = windows.NewLazySystemDLL("shell32.dll")

// Explorer copies file locations as CF_HDROP, not clipboard text or browser File objects.
func (a *App) GetClipboardFiles() ([]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	available, _, _ := clipboardUser.NewProc("IsClipboardFormatAvailable").Call(15)
	if available == 0 {
		return []string{}, nil
	}
	opened, _, _ := clipboardUser.NewProc("OpenClipboard").Call(0)
	if opened == 0 {
		return nil, fmt.Errorf("剪贴板正被占用，请重试粘贴")
	}
	defer clipboardUser.NewProc("CloseClipboard").Call()
	handle, _, _ := clipboardUser.NewProc("GetClipboardData").Call(15)
	if handle == 0 {
		return nil, fmt.Errorf("无法读取剪贴板文件")
	}
	query := clipboardShell.NewProc("DragQueryFileW")
	count, _, _ := query.Call(handle, 0xffffffff, 0, 0)
	if count > 4096 {
		return nil, fmt.Errorf("一次最多粘贴 4096 个文件或文件夹")
	}
	paths := make([]string, 0, int(count))
	for i := uintptr(0); i < count; i++ {
		length, _, _ := query.Call(handle, i, 0, 0)
		if length == 0 || length > 32767 {
			return nil, fmt.Errorf("剪贴板中的文件路径无效")
		}
		buffer := make([]uint16, int(length)+1)
		read, _, _ := query.Call(handle, i, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
		if read != length {
			return nil, fmt.Errorf("无法读取完整的剪贴板文件路径")
		}
		paths = append(paths, windows.UTF16ToString(buffer))
	}
	return paths, nil
}
