package shub

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

type stateLock struct {
	file *os.File
}

func (manager *Manager) withStateLock(fn func() error) error {
	lock, err := manager.acquireStateLock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close() }()
	return fn()
}

func (manager *Manager) acquireStateLock() (*stateLock, error) {
	if err := os.MkdirAll(manager.homeRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create SHUB home for lock: %w", err)
	}

	lockPath := filepath.Join(manager.homeRoot, ".lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open SHUB lock file: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire SHUB lock: %w", err)
	}
	return &stateLock{file: file}, nil
}

func (lock *stateLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := syscall.Flock(int(lock.file.Fd()), syscall.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	return errors.Join(unlockErr, closeErr)
}
