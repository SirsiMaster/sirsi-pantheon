//go:build !windows

package selfupdate

import (
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
