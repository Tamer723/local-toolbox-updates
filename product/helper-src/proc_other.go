//go:build !windows

package main

import (
	"errors"
	"syscall"
)

func hiddenProcessAttributes() *syscall.SysProcAttr { return nil }

func openNativePath(path string, isDir bool) error {
	return errors.New("open path is only implemented for Windows")
}
