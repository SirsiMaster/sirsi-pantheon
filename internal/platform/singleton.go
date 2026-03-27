// Package platform — singleton.go
// 𓁟 Pantheon Platform Governance — Rule A1
package platform

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// TryLock attempts to acquire a singleton lock for the given ID.
// Returns a cleanup function and nil on success.
// If another instance is already running, returns a descriptive error.
func TryLock(id string) (func(), error) {
	// We use Unix domain sockets for locks on macOS/Linux.
	// This is more robust than PID files as the OS cleans them up on crash.
	lockPath := filepath.Join(os.TempDir(), fmt.Sprintf("pantheon.%s.lock", id))

	// Attempt to listen on the socket. If it fails, another instance is running.
	l, err := net.Listen("unix", lockPath)
	if err != nil {
		// Verify if the socket is actually active
		_, dialErr := net.Dial("unix", lockPath)
		if dialErr == nil {
			return nil, fmt.Errorf("another instance of %s is already running (locked at %s)", id, lockPath)
		}

		// If dial fails, the socket might be stale. Clean it up.
		_ = os.Remove(lockPath)
		l, err = net.Listen("unix", lockPath)
		if err != nil {
			return nil, fmt.Errorf("failed to acquire lock for %s: %w", id, err)
		}
	}

	cleanup := func() {
		_ = l.Close()
		_ = os.Remove(lockPath)
	}

	return cleanup, nil
}
