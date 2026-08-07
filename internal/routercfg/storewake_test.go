package routercfg

import (
	"os"
	"path/filepath"
	"testing"
)

// Env always wins, in both directions.
func TestStoreWakeEnvWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // no marker
	t.Setenv(StoreWakeEnv, "1")
	if !StoreWake() {
		t.Error("env=1 with no marker must be ON")
	}
	t.Setenv(StoreWakeEnv, "0")
	if StoreWake() {
		t.Error("env=0 must be OFF even if a marker exists")
	}
}

// A genuinely absent marker is the ONE case that means "not cut over".
func TestStoreWakeAbsentMarkerIsOff(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	os.Unsetenv(StoreWakeEnv)
	if StoreWake() {
		t.Error("absent marker must read as not-cut-over")
	}
}

func TestStoreWakePresentMarkerIsOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv(StoreWakeEnv)
	if err := os.MkdirAll(filepath.Join(home, ".sirsi"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, markerRel), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !StoreWake() {
		t.Error("present marker must read as cut over")
	}
}

// THE regression. An UNREADABLE marker is UNKNOWN, and unknown must never
// resolve to "off" — that direction sends writes to the legacy threads.json and
// mints a silent second thread registry that no code path reconciles.
// Measured 2026-08-07: a sandboxed codex-assiduous session took exactly this
// branch and attempted a legacy write ("create temp threads.json: … operation
// not permitted"). It surfaced only because the sandbox ALSO denied the write.
func TestStoreWakeUnreadableMarkerDoesNotDowngrade(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions, so the denial cannot be staged")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	os.Unsetenv(StoreWakeEnv)
	dir := filepath.Join(home, ".sirsi")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, markerRel), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Strip traverse permission so stat fails with EACCES, not ENOENT — the
	// shape a sandbox produces.
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	if _, err := os.Stat(filepath.Join(home, markerRel)); err == nil || os.IsNotExist(err) {
		t.Skipf("could not stage an unreadable marker on this filesystem (err=%v)", err)
	}
	if !StoreWake() {
		t.Error("an UNREADABLE marker downgraded to legacy writes — this is how a silent second thread registry gets written")
	}
}

// $HOME unknown is the same unknown and takes the same direction.
func TestStoreWakeNoHomeDoesNotDowngrade(t *testing.T) {
	t.Setenv("HOME", "")
	os.Unsetenv(StoreWakeEnv)
	if MarkerPath() != "" {
		t.Skip("this platform resolves a home directory without $HOME")
	}
	if !StoreWake() {
		t.Error("unknown $HOME downgraded to legacy writes")
	}
}
