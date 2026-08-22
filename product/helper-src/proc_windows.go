//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

func hiddenProcessAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

var (
	shell32Open       = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW = shell32Open.NewProc("ShellExecuteW")
)

func openNativePath(path string, isDir bool) error {
	verb, _ := syscall.UTF16PtrFromString("open")
	show := uintptr(1) // SW_SHOWNORMAL
	var file, params *uint16
	if isDir {
		file, _ = syscall.UTF16PtrFromString(path)
	} else {
		file, _ = syscall.UTF16PtrFromString("explorer.exe")
		// Explorer expects /select,<path> as a single parameter string. Quoting
		// the path handles spaces and non-ASCII filenames reliably.
		paramText := `/select,"` + path + `"`
		params, _ = syscall.UTF16PtrFromString(paramText)
	}
	ret, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		uintptr(unsafe.Pointer(params)),
		0,
		show,
	)
	if ret <= 32 {
		return fmt.Errorf("Windows ShellExecute failed with code %d", ret)
	}
	return nil
}
