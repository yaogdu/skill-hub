package shub

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type stateLock struct {
	path string
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

	lockPath := filepath.Join(manager.homeRoot, ".state.lock")
	for {
		if err := os.Mkdir(lockPath, 0o700); err == nil {
			return &stateLock{path: lockPath}, nil
		} else if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire SHUB lock: %w", err)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func (lock *stateLock) Close() error {
	if lock == nil || lock.path == "" {
		return nil
	}
	err := os.Remove(lock.path)
	lock.path = ""
	return err
}
