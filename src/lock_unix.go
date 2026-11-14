//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireProcessLock(name string) (*ProcessLock, error) {
	lockPath := filepath.Join(getLockDir(), fmt.Sprintf("%s.lock", name))
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("another instance of autologin is already running")
	}

	return &ProcessLock{file: f, path: lockPath}, nil
}

func (l *ProcessLock) Release() {
	if l != nil && l.file != nil {
		_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
		_ = l.file.Close()
		_ = os.Remove(l.path)
	}
}
