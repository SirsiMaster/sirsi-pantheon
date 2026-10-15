//go:build unix

// Package router — admission_unix.go
//
// The admitConsumer critical section's lock primitive. A blocking (not
// LOCK_NB) exclusive flock(2): unlike internal/selfupdate's install lock,
// which fails loudly when already held, a second admission caller should
// WAIT for the first to finish and then adopt what it dispatched — never
// bail out. The file is created once and never removed, so every locker
// across the process's lifetime arbitrates on the same inode (see
// internal/selfupdate/lock_unix.go's inode-recycling note).
package router

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockConsumerAdmission blocks until it holds an exclusive flock on path,
// creating the file (and its parent dir) if needed. The returned func
// releases the lock and closes the fd; the caller must call it exactly once.
func lockConsumerAdmission(path string) (unlock func(), err error) {
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o755); mkErr != nil {
		return nil, fmt.Errorf("create admission lock dir: %w", mkErr)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open admission lock %s: %w", path, err)
	}
	if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); flockErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock admission lock %s: %w", path, flockErr)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
