package stealth

import (
	"os"
	"path/filepath"
	"testing"
)

// helper creates a fake home directory with Anubis artifacts.
func setupFakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()

	// Create files that CleanExit should remove
	dirs := []string{
		filepath.Join(home, ".config", "anubis", "profiles"),
		filepath.Join(home, ".anubis", "weights"),
		filepath.Join(home, ".cache", "anubis"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Drop marker files
	files := []string{
		filepath.Join(home, ".config", "anubis", "config.yaml"),
		filepath.Join(home, ".config", "anubis", "profiles", "developer.yaml"),
		filepath.Join(home, ".anubis", "weights", "model.bin"),
		filepath.Join(home, ".cache", "anubis", "scan-cache.json"),
	}
	for _, f := range files {
		if err := os.WriteFile(f, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	return home
}

// ─────────────────────────────────────────────────
// CleanExit
// ─────────────────────────────────────────────────

func TestCleanExit_RemovesAllTargets(t *testing.T) {
	home := setupFakeHome(t)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanExit(); err != nil {
		t.Fatalf("CleanExit() error: %v", err)
	}

	targets := []string{
		filepath.Join(home, ".config", "anubis"),
		filepath.Join(home, ".anubis"),
		filepath.Join(home, ".cache", "anubis"),
	}
	for _, target := range targets {
		if _, err := os.Stat(target); err == nil {
			t.Errorf("CleanExit() did not remove %s", target)
		}
	}
}

func TestCleanExit_NoErrorWhenTargetsMissing(t *testing.T) {
	home := t.TempDir() // clean — no Anubis dirs
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanExit(); err != nil {
		t.Fatalf("CleanExit() should succeed when targets don't exist, got: %v", err)
	}
}

func TestCleanExit_PreservesOtherConfigs(t *testing.T) {
	home := setupFakeHome(t)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	// Create a sibling config that should NOT be touched
	otherConfig := filepath.Join(home, ".config", "other-app", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(otherConfig), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherConfig, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CleanExit(); err != nil {
		t.Fatalf("CleanExit() error: %v", err)
	}

	// Other config must survive
	if _, err := os.Stat(otherConfig); err != nil {
		t.Errorf("CleanExit() removed unrelated config: %s", otherConfig)
	}
}

// ─────────────────────────────────────────────────
// CleanBrain
// ─────────────────────────────────────────────────

func TestCleanBrain_RemovesWeights(t *testing.T) {
	home := setupFakeHome(t)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanBrain(); err != nil {
		t.Fatalf("CleanBrain() error: %v", err)
	}

	weightsDir := filepath.Join(home, ".anubis", "weights")
	if _, err := os.Stat(weightsDir); err == nil {
		t.Error("CleanBrain() did not remove weights directory")
	}

	// .anubis root should still exist (only weights removed)
	anubisDir := filepath.Join(home, ".anubis")
	if _, err := os.Stat(anubisDir); err != nil {
		t.Error("CleanBrain() should not remove parent .anubis directory")
	}
}

func TestCleanBrain_NoErrorWhenMissing(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanBrain(); err != nil {
		t.Fatalf("CleanBrain() should succeed when weights don't exist, got: %v", err)
	}
}

// ─────────────────────────────────────────────────
// CleanCache
// ─────────────────────────────────────────────────

func TestCleanCache_RemovesCacheDir(t *testing.T) {
	home := setupFakeHome(t)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanCache(); err != nil {
		t.Fatalf("CleanCache() error: %v", err)
	}

	cacheDir := filepath.Join(home, ".cache", "anubis")
	if _, err := os.Stat(cacheDir); err == nil {
		t.Error("CleanCache() did not remove cache directory")
	}

	// .cache root should still exist
	cacheRoot := filepath.Join(home, ".cache")
	if _, err := os.Stat(cacheRoot); err != nil {
		t.Error("CleanCache() should not remove parent .cache directory")
	}
}

func TestCleanCache_NoErrorWhenMissing(t *testing.T) {
	home := t.TempDir()
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanCache(); err != nil {
		t.Fatalf("CleanCache() should succeed when cache doesn't exist, got: %v", err)
	}
}

// ─────────────────────────────────────────────────
// Isolation checks
// ─────────────────────────────────────────────────

func TestCleanBrain_DoesNotAffectConfig(t *testing.T) {
	home := setupFakeHome(t)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanBrain(); err != nil {
		t.Fatalf("CleanBrain() error: %v", err)
	}

	// Config should still be intact
	configFile := filepath.Join(home, ".config", "anubis", "config.yaml")
	if _, err := os.Stat(configFile); err != nil {
		t.Error("CleanBrain() should not touch config files")
	}
}

func TestCleanCache_DoesNotAffectWeights(t *testing.T) {
	home := setupFakeHome(t)
	origHome := os.Getenv("HOME")
	t.Setenv("HOME", home)
	defer os.Setenv("HOME", origHome)

	if err := CleanCache(); err != nil {
		t.Fatalf("CleanCache() error: %v", err)
	}

	// Weights should still be intact
	weightsFile := filepath.Join(home, ".anubis", "weights", "model.bin")
	if _, err := os.Stat(weightsFile); err != nil {
		t.Error("CleanCache() should not touch brain weights")
	}
}
