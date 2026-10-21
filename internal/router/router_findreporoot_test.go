package router

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// sameDir reports whether two paths resolve to the same directory, tolerating
// macOS's /var → /private/var symlink (t.TempDir vs os.Getwd disagree on it).
func sameDir(t *testing.T, a, b string) bool {
	t.Helper()
	ai, err := os.Stat(a)
	if err != nil {
		t.Fatalf("stat %s: %v", a, err)
	}
	bi, err := os.Stat(b)
	if err != nil {
		t.Fatalf("stat %s: %v", b, err)
	}
	return os.SameFile(ai, bi)
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// The router must remain canonical, while a completion proof must be bound to
// the linked worktree that actually invoked the close. These are intentionally
// distinct roots.
func TestFindCallerRepoRootUsesActualLinkedWorktree(t *testing.T) {
	base := t.TempDir()
	mainRoot := filepath.Join(base, "router")
	appRoot := filepath.Join(base, "application-main")
	worktree := filepath.Join(base, "application-worktree")
	if err := os.MkdirAll(mainRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, mainRoot, "init", "-q")
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, appRoot, "init", "-q")
	gitRun(t, appRoot, "config", "user.email", "tests@example.invalid")
	gitRun(t, appRoot, "config", "user.name", "tests")
	if err := os.WriteFile(filepath.Join(appRoot, "README"), []byte("application\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, appRoot, "add", "README")
	gitRun(t, appRoot, "commit", "-qm", "initial")
	gitRun(t, appRoot, "worktree", "add", "-q", "-b", "application", worktree, "HEAD")
	if err := os.MkdirAll(filepath.Join(worktree, "nested", "cwd"), 0o755); err != nil {
		t.Fatal(err)
	}
	mkRouter(t, mainRoot)

	restore := setGitCommonDirFn(func() (string, bool) {
		return filepath.Join(mainRoot, ".git"), true
	})
	t.Cleanup(func() { setGitCommonDirFn(restore) })
	t.Chdir(filepath.Join(worktree, "nested", "cwd"))

	routerRoot, err := FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	if !sameDir(t, routerRoot, mainRoot) {
		t.Fatalf("router root = %q, want canonical %q", routerRoot, mainRoot)
	}
	callerRoot, err := FindCallerRepoRoot()
	if err != nil {
		t.Fatalf("FindCallerRepoRoot: %v", err)
	}
	if !sameDir(t, callerRoot, worktree) {
		t.Fatalf("caller root = %q, want linked worktree %q", callerRoot, worktree)
	}
}

func TestFindCallerRepoRootRefusesUnknownCaller(t *testing.T) {
	t.Chdir(t.TempDir())
	if root, err := FindCallerRepoRoot(); err == nil {
		t.Fatalf("FindCallerRepoRoot = %q, want unknown caller refusal", root)
	}
}

func mkRouter(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// FindRepoRoot must prefer the MAIN worktree root (shared .git's parent) over a
// per-agent worktree's stale snapshot copy — the ADR-029 canonical-router fix.
func TestFindRepoRoot_PrefersCanonicalGitRoot(t *testing.T) {
	tmp := t.TempDir()
	mainRoot := filepath.Join(tmp, "main")
	worktree := filepath.Join(tmp, "main", ".claude", "worktrees", "wt")
	mkRouter(t, mainRoot) // the one live router
	mkRouter(t, worktree) // the stale snapshot the cwd walk-up would wrongly find

	// Simulate `git rev-parse --git-common-dir` from inside the worktree:
	// it returns the MAIN tree's .git, regardless of cwd.
	restore := setGitCommonDirFn(func() (string, bool) {
		return filepath.Join(mainRoot, ".git"), true
	})
	defer func() { setGitCommonDirFn(restore) }()

	t.Chdir(worktree) // run "from" the worktree

	got, err := FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	if got != mainRoot {
		t.Errorf("FindRepoRoot from worktree = %q, want canonical main root %q", got, mainRoot)
	}
}

// With no git tree, FindRepoRoot falls back to the cwd walk-up (unchanged behavior).
func TestFindRepoRoot_FallsBackToWalkUpWhenNoGit(t *testing.T) {
	tmp := t.TempDir()
	mkRouter(t, tmp)
	sub := filepath.Join(tmp, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	restore := setGitCommonDirFn(func() (string, bool) { return "", false })
	defer func() { setGitCommonDirFn(restore) }()

	t.Chdir(sub)

	got, err := FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	if !sameDir(t, got, tmp) {
		t.Errorf("FindRepoRoot = %q, want walk-up root %q", got, tmp)
	}
}

// If git resolves but the canonical root has no router (e.g. unusual git dir),
// FindRepoRoot must NOT return it — it falls through to the cwd walk-up.
func TestFindRepoRoot_FallsBackWhenGitRootHasNoRouter(t *testing.T) {
	tmp := t.TempDir()
	cwdRoot := filepath.Join(tmp, "cwd")
	gitRoot := filepath.Join(tmp, "elsewhere") // has .git but no .agents/idea-router
	mkRouter(t, cwdRoot)
	if err := os.MkdirAll(gitRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	restore := setGitCommonDirFn(func() (string, bool) {
		return filepath.Join(gitRoot, ".git"), true
	})
	defer func() { setGitCommonDirFn(restore) }()

	t.Chdir(cwdRoot)

	got, err := FindRepoRoot()
	if err != nil {
		t.Fatalf("FindRepoRoot: %v", err)
	}
	if !sameDir(t, got, cwdRoot) {
		t.Errorf("FindRepoRoot = %q, want walk-up root %q (git root had no router)", got, cwdRoot)
	}
}
