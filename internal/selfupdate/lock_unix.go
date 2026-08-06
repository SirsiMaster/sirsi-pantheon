//go:build !windows

// Package selfupdate — lock_unix.go
//
// Nonblocking, loud advisory install lock. Prevents concurrent `sirsi
// self-update --confirm` runs from racing through SafeReplace on the same
// target binaries. The lock is a kernel flock(2) so it is automatically
// released if the process crashes or is killed — no stale-lock cleanup needed.
//
// The lock file body stores the holder PID so a concurrent caller can report
// it clearly. The file is kept at ~/.sirsi/binary-install.lock (same dir as
// the rest of the sirsi state).
package selfupdate

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const lockFile = "binary-install.lock"

func installLockPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".sirsi", lockFile), nil
}

// AcquireInstallLock tries to take an exclusive flock on
// ~/.sirsi/binary-install.lock without blocking. On success it returns the
// open file; the caller must close it (or defer f.Close()) to release.
//
// On failure it returns a loud error. If the lock file contains a PID, the
// error reports that it is recorded without presenting unverified kill advice.
func AcquireInstallLock() (*os.File, error) {
	return acquireInstallLockWith(writeLockPID)
}

func acquireInstallLockWith(recordPID func(*os.File) error) (*os.File, error) {
	path, err := installLockPath()
	if err != nil {
		return nil, fmt.Errorf("install lock: %w", err)
	}
	if mkErr := os.MkdirAll(filepath.Dir(path), 0o700); mkErr != nil {
		return nil, fmt.Errorf("install lock: create dir: %w", mkErr)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, fmt.Errorf("install lock: open %s: %w", path, err)
	}
	if flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); flockErr != nil {
		holder := readLockPID(f)
		f.Close()
		if holder != "" {
			return nil, fmt.Errorf(
				"sirsi self-update is already running (PID %s is recorded in %s) — wait for it to finish; verify process identity before taking action",
				holder, path)
		}
		return nil, fmt.Errorf(
			"sirsi self-update is already running (lock: %s) — wait for it to finish", path)
	}
	if recordErr := recordPID(f); recordErr != nil {
		_ = f.Close()
		return nil, fmt.Errorf("install lock: record holder PID: %w", recordErr)
	}
	return f, nil
}

func writeLockPID(f *os.File) error {
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("truncate PID record: %w", err)
	}
	if _, err := f.WriteString(strconv.Itoa(os.Getpid())); err != nil {
		return fmt.Errorf("write PID record: %w", err)
	}
	return nil
}

func readLockPID(f *os.File) string {
	if _, err := f.Seek(0, 0); err != nil {
		return ""
	}
	buf := make([]byte, 32)
	n, _ := f.Read(buf)
	return strings.TrimSpace(string(buf[:n]))
}
