package router

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeGitOnPath puts a `git` script first on PATH. The script is run by sh and
// (for the hang case) spawns a child sleep that inherits stdout, so killing sh
// alone would leave the pipe open: this exercises WaitDelay too.
func fakeGitOnPath(t *testing.T, script string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// runWithDeadline fails the test (instead of hanging the suite) if fn does not
// return within limit.
func runWithDeadline[T any](t *testing.T, limit time.Duration, fn func() T) T {
	t.Helper()
	ch := make(chan T, 1)
	go func() { ch <- fn() }()
	select {
	case v := <-ch:
		return v
	case <-time.After(limit):
		t.Fatalf("did not return within %s: git probe is unbounded", limit)
		panic("unreachable")
	}
}

type probeResult struct {
	dir string
	ok  bool
}

func TestGitCommonDirWithin_HangingGitIsBounded(t *testing.T) {
	fakeGitOnPath(t, "sleep 30; echo /nonexistent/.git")
	start := time.Now()
	r := runWithDeadline(t, 5*time.Second, func() probeResult {
		d, ok := gitCommonDirWithin(150 * time.Millisecond)
		return probeResult{d, ok}
	})
	if r.ok {
		t.Fatalf("hung git must report not-ok, got %q", r.dir)
	}
	if el := time.Since(start); el > 4*time.Second {
		t.Fatalf("took %s, want well under 4s", el)
	}
}

func TestGitCommonDirWithin_FastGitUnchanged(t *testing.T) {
	fakeGitOnPath(t, "echo /some/main/.git")
	d, ok := gitCommonDirWithin(5 * time.Second)
	if !ok || d != "/some/main/.git" {
		t.Fatalf("got (%q,%v), want (/some/main/.git,true)", d, ok)
	}
}

func TestGitCommonDirWithin_GitErrorFallsThrough(t *testing.T) {
	fakeGitOnPath(t, "exit 128")
	if d, ok := gitCommonDirWithin(5 * time.Second); ok {
		t.Fatalf("failing git must report not-ok, got %q", d)
	}
}

// A git that hangs must not stop FindRepoRoot from resolving via the cwd walk-up.
func TestFindRepoRoot_HangingGitFallsBackToWalkUp(t *testing.T) {
	isolateGitEnv(t)
	root := scaffoldClone(t)
	fakeGitOnPath(t, "sleep 30")
	restore := setGitCommonDirFn(func() (string, bool) { return gitCommonDirWithin(150 * time.Millisecond) })
	t.Cleanup(func() { setGitCommonDirFn(restore) })
	t.Chdir(root)

	type res struct {
		root string
		err  error
	}
	r := runWithDeadline(t, 5*time.Second, func() res {
		g, err := FindRepoRoot()
		return res{g, err}
	})
	if r.err != nil {
		t.Fatalf("FindRepoRoot: %v", r.err)
	}
	if !sameDir(t, r.root, root) {
		t.Fatalf("FindRepoRoot = %q, want walk-up root %q", r.root, root)
	}
}
