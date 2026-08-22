//go:build !windows

package main

import "errors"

func pickMediaFile() (string, bool, error) {
	return "", false, errors.New("file picker is only available on Windows")
}
func pickFolder() (string, bool, error) {
	return "", false, errors.New("folder picker is only available on Windows")
}
