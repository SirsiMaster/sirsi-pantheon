//go:build !windows

package routerstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

// lockSpoolAdmission locks the retained spool DIRECTORY, not a removable marker.
// SH covers publication through response settlement; EX prevents updated clients
// publishing while the selected relay is refreshed. This is cooperative: legacy
// writers do not honor it and existing queue/instance guards remain mandatory.
func lockSpoolAdmission(ctx context.Context, root *os.Root, exclusive bool) (*os.File, error) {
	f, err := root.Open(".")
	if err != nil {
		return nil, err
	}
	mode := unix.LOCK_SH
	if exclusive {
		mode = unix.LOCK_EX
	}
	for {
		if err = ctx.Err(); err != nil {
			_ = f.Close()
			return nil, err
		}
		err = unix.Flock(int(f.Fd()), mode|unix.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = f.Close()
			return nil, fmt.Errorf("spool admission lock: %w", err)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = f.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// WithSpoolQuiescence waits at most five seconds for updated in-flight clients,
// then retains exclusive producer admission through operation (including its
// existing finite recovery). It never cancels/replays a published request. The
// caller must use a bounded operation and verify all legacy producers separately.
// Perform operator RPC authorization BEFORE entry and RPC readback AFTER return:
// a spool RPC inside operation would correctly block on its own exclusive lock.
func WithSpoolQuiescence(ctx context.Context, physical string, operation func() error) error {
	if operation == nil {
		return errors.New("spool admission: operation required")
	}
	root, err := os.OpenRoot(physical)
	if err != nil {
		return err
	}
	defer root.Close()
	before, err := os.Lstat(physical)
	if err != nil || !before.IsDir() {
		return errors.New("spool admission: real directory required")
	}
	wait, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	lock, err := lockSpoolAdmission(wait, root, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	held, err := lock.Stat()
	if err != nil {
		return err
	}
	after, err := os.Lstat(physical)
	if err != nil || !os.SameFile(before, held) || !os.SameFile(held, after) {
		return errors.New("spool admission: root identity changed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation()
}
