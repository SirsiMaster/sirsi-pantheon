// Package router — reporootmarker.go
//
// App-context repo root (adversarial review of #147, blocker B2): every
// menubar lever shells `sirsi router ...`, which resolves the clone through
// FindRepoRoot — a git probe plus a cwd walk-up. A LaunchAgent-spawned app has
// cwd=/ (or $HOME), neither of which is inside the clone, so every lever died
// outside a dev shell. The fix is a persisted marker: root-ful invocations
// (sirsi setup, agent register, supervisor start) record the clone root at
// ~/.sirsi/pantheon-repo, and FindRepoRoot reads it as a LAST resort — the
// git probe and the cwd walk-up always win when they resolve, so dev shells
// and tests are unaffected.
package router

import (
	"os"
	"path/filepath"
	"strings"
)

// repoRootMarkerEnv overrides the marker file location (tests, harnesses).
// Production leaves it unset → ~/.sirsi/pantheon-repo.
const repoRootMarkerEnv = "SIRSI_PANTHEON_REPO_MARKER"

// repoRootMarkerPath resolves the marker file location.
func repoRootMarkerPath() string {
	if p := strings.TrimSpace(os.Getenv(repoRootMarkerEnv)); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".sirsi", "pantheon-repo")
}

// validRepoRoot reports whether root actually contains the live router — the
// same anchor FindRepoRoot's walk-up trusts. A marker is only ever honored
// when this holds, so a stale or hand-edited marker can never send a caller
// into a directory without a router.
func validRepoRoot(root string) bool {
	if root == "" || !filepath.IsAbs(root) {
		return false
	}
	info, err := os.Stat(filepath.Join(root, ".agents", "idea-router"))
	return err == nil && info.IsDir()
}

// RememberRepoRoot persists the clone root for app-context callers. Best
// effort and write-guarded: an invalid root is never written, and an
// already-correct marker is left untouched (no write churn — Spotlight
// write-amplification class). Call it wherever a root-ful invocation has
// just resolved the real clone (setup, agent register, supervisor start).
func RememberRepoRoot(root string) {
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}
	if !validRepoRoot(root) {
		return
	}
	path := repoRootMarkerPath()
	if path == "" {
		return
	}
	if existing, err := os.ReadFile(path); err == nil && strings.TrimSpace(string(existing)) == root {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(path, []byte(root+"\n"), 0o644)
}

// repoRootFromMarker reads and validates the persisted root. ("", false) when
// the marker is absent, unreadable, or points at a directory that no longer
// carries .agents/idea-router (a moved or deleted clone).
func repoRootFromMarker() (string, bool) {
	path := repoRootMarkerPath()
	if path == "" {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	root := strings.TrimSpace(string(data))
	if !validRepoRoot(root) {
		return "", false
	}
	return root, true
}
