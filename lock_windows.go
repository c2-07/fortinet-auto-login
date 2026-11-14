//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func acquireProcessLock(name string) (*ProcessLock, error) {
	lockPath := filepath.Join(getLockDir(), fmt.Sprintf("%s.lock", name))
	pathPtr, err := syscall.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, err
	}

	handle, err := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		0, // No sharing -> exclusive lock
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("another instance of autologin is already running")
	}

	f := os.NewFile(uintptr(handle), lockPath)
	return &ProcessLock{file: f, path: lockPath}, nil
}

func (l *ProcessLock) Release() {
	if l != nil && l.file != nil {
		_ = l.file.Close()
		_ = os.Remove(l.path)
	}
}
