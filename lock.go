package main

import (
	"os"
	"path/filepath"
)

type ProcessLock struct {
	file *os.File
	path string
}

func getLockDir() string {
	dir := filepath.Join(os.TempDir(), "fortinet-autologin-locks")
	_ = os.MkdirAll(dir, 0700)
	return dir
}
