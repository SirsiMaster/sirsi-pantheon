//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// TryLockMenubarSurface joins the legacy Go surface to the same descriptor-held
// per-user lease used by the shipped SwiftUI menubar.  It deliberately does not
// use TryLock("menubar"): that older /tmp socket was a separate namespace, so
// a legacy binary and Pantheon.app could both add a status item.
func TryLockMenubarSurface() (func(), error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("resolve menubar support directory: %w", err)
	}
	directory := filepath.Join(home, "Library", "Application Support", "Sirsi", "Pantheon")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, fmt.Errorf("create menubar support directory: %w", err)
	}
	return tryLockMenubarLease(filepath.Join(directory, "menubar.instance.lock"))
}

// tryLockMenubarLease is package-visible only to give the Darwin contract test
// an isolated pathname. Production derives its location through
// TryLockMenubarSurface so another user or Mac can never contend for this lease.
func tryLockMenubarLease(path string) (func(), error) {
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open menubar instance lease: %w", err)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("Pantheon menubar is already running: %w", err)
	}
	return func() {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = unix.Close(fd)
	}, nil
}
