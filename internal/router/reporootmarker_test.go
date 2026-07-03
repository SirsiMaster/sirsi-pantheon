package router

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldClone creates a temp dir carrying .agents/idea-router (the anchor
// both the walk-up and the marker validation trust).
func scaffoldClone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Resolve symlinks (macOS t.TempDir lives under /var → /private/var) so
	// equality assertions compare canonical paths.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	return root
}

// chdirNoRouter moves cwd to a temp dir with no router anywhere above it and
// stubs the git probe to "not a git tree", so only the marker can resolve.
func chdirNoRouter(t *testing.T) {
	t.Helper()
	orig := setGitCommonDirFn(func() (string, bool) { return "", false })
	t.Cleanup(func() { setGitCommonDirFn(orig) })
	t.Chdir(t.TempDir())
}

// TestFindRepoRoot_MarkerIsLastResort is the app-context regression test
// (blocker B2 of the #147 adversarial review): with no git tree and no router
// on the cwd walk-up — a launchd-spawned menubar lever — FindRepoRoot must
// resolve through the persisted marker.
func TestFindRepoRoot_MarkerIsLastResort(t *testing.T) {
	clone := scaffoldClone(t)
	marker := filepath.Join(t.TempDir(), "pantheon-repo")
	if err := os.WriteFile(marker, []byte(clone+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(repoRootMarkerEnv, marker)
	chdirNoRouter(t)

	got, err := FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot should resolve via the marker: %v", err)
	}
	if got != clone {
		t.Errorf("FindRepoRoot = %q, want marker root %q", got, clone)
	}
}

// TestFindRepoRoot_WalkUpBeatsMarker proves precedence: a live router on the
// walk-up always wins over the marker, so dev shells never get redirected.
func TestFindRepoRoot_WalkUpBeatsMarker(t *testing.T) {
	markerClone := scaffoldClone(t)
	cwdClone := scaffoldClone(t)
	marker := filepath.Join(t.TempDir(), "pantheon-repo")
	if err := os.WriteFile(marker, []byte(markerClone+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(repoRootMarkerEnv, marker)
	orig := setGitCommonDirFn(func() (string, bool) { return "", false })
	t.Cleanup(func() { setGitCommonDirFn(orig) })
	t.Chdir(cwdClone)

	got, err := FindRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != cwdClone {
		t.Errorf("FindRepoRoot = %q, want cwd walk-up root %q (marker must be LAST resort)", got, cwdClone)
	}
}

// TestFindRepoRoot_StaleMarkerRejected proves the read-side validation: a
// marker pointing at a directory without .agents/idea-router (moved/deleted
// clone) is ignored and the original error surfaces.
func TestFindRepoRoot_StaleMarkerRejected(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pantheon-repo")
	if err := os.WriteFile(marker, []byte(t.TempDir()+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(repoRootMarkerEnv, marker)
	chdirNoRouter(t)

	if got, err := FindRepoRoot(); err == nil {
		t.Errorf("FindRepoRoot = %q, want error — a stale marker must never resolve", got)
	} else if !strings.Contains(err.Error(), "idea-router") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestRememberRepoRoot_WritesValidatesAndGuards covers the write side:
// a valid root is persisted once (idempotent re-write leaves mtime alone is
// not asserted — content is), and an invalid root is never written.
func TestRememberRepoRoot_WritesValidatesAndGuards(t *testing.T) {
	clone := scaffoldClone(t)
	marker := filepath.Join(t.TempDir(), "nested", "pantheon-repo")
	t.Setenv(repoRootMarkerEnv, marker)

	RememberRepoRoot(clone)
	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("marker not written: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != clone {
		t.Errorf("marker = %q, want %q", got, clone)
	}

	// An invalid root (no .agents/idea-router) must NOT clobber the marker.
	RememberRepoRoot(t.TempDir())
	data, _ = os.ReadFile(marker)
	if got := strings.TrimSpace(string(data)); got != clone {
		t.Errorf("invalid root overwrote the marker: %q", got)
	}
}
