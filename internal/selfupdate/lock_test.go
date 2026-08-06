//go:build !windows

package selfupdate

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// setHome points os.UserHomeDir() at dir for the duration of the test by
// setting $HOME. Works because Go stdlib reads $HOME on Unix before passwd.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
}

func TestAcquireInstallLock_basic(t *testing.T) {
	setHome(t, t.TempDir())

	f, err := AcquireInstallLock()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if f == nil {
		t.Fatal("expected non-nil file on success")
	}
	defer f.Close()
}

func TestAcquireInstallLock_lockFileCreated(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	f, err := AcquireInstallLock()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer f.Close()

	path := filepath.Join(home, ".sirsi", lockFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file not created at %s: %v", path, err)
	}
}

func TestAcquireInstallLock_writesPID(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	f, err := AcquireInstallLock()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer f.Close()

	raw, err := os.ReadFile(filepath.Join(home, ".sirsi", lockFile))
	if err != nil {
		t.Fatalf("read lock file: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatalf("lock file does not contain a PID: %q", string(raw))
	}
	if pid != os.Getpid() {
		t.Fatalf("lock file PID %d != os.Getpid() %d", pid, os.Getpid())
	}
}

// TestAcquireInstallLock_pidTruncatedAfterClose verifies that Close() truncates
// the PID to empty rather than removing the file. Truncating keeps the inode
// live so any concurrent opener that already has a descriptor on the same inode
// will flock the same inode — not a freshly-created one — preserving kernel
// arbitration. Shell's acquire_lock() empty-PID reap branch removes the file
// on the next acquire.
func TestAcquireInstallLock_pidTruncatedAfterClose(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	lock, err := AcquireInstallLock()
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	path := filepath.Join(home, ".sirsi", lockFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("lock file must exist before close: %v", err)
	}
	lock.Close()
	// File stays (not unlinked) but content must be empty so shell reap handles it.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("lock file must still exist after close (truncate, not unlink): %v", err)
	}
	if len(raw) != 0 {
		t.Fatalf("lock file must be empty after close; got %q", string(raw))
	}
}

func TestAcquireInstallLock_releasedAfterClose(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)

	f1, err := AcquireInstallLock()
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	// Same-process flock upgrades (not errors) on the same file, so we just
	// verify close doesn't panic and the file still exists.
	f1.Close()

	f2, err := AcquireInstallLock()
	if err != nil {
		t.Fatalf("re-acquire after release: %v", err)
	}
	defer f2.Close()
}

func TestReadLockPID_empty(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "lock-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := readLockPID(f); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestReadLockPID_withPID(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "lock-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("9999\n")
	f.Seek(0, 0)
	if got := readLockPID(f); got != "9999" {
		t.Fatalf("expected 9999, got %q", got)
	}
}

func TestAcquireInstallLock_PIDRecordFailureNeverAdvisesKillingStalePID(t *testing.T) {
	home := t.TempDir()
	setHome(t, home)
	path := filepath.Join(home, ".sirsi", lockFile)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	const stalePID = "424242"
	if err := os.WriteFile(path, []byte(stalePID), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := acquireInstallLockWith(func(*os.File) error {
		return errors.New("forced PID-record write failure")
	})
	if f != nil {
		_ = f.Close()
		t.Fatal("failed PID record must not return a held install lock")
	}
	if err == nil {
		t.Fatal("forced PID-record write failure returned nil error")
	}
	msg := err.Error()
	if strings.Contains(msg, stalePID) || strings.Contains(msg, "`kill") {
		t.Fatalf("failure emitted stale/recycled PID kill advice: %q", msg)
	}
	if !strings.Contains(msg, "record holder PID") {
		t.Fatalf("failure did not identify the PID-record boundary: %q", msg)
	}
}
