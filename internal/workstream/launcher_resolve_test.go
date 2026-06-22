package workstream

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestResolveBinary_FindsToolOutsidePATH is the regression test for the menubar
// detection bug: under a launchd-truncated PATH (no ~/.local/bin), a bare
// exec.LookPath misses the claude/codex CLIs that ARE installed. resolveBinary
// must still find them via the augmented candidate dirs.
func TestResolveBinary_FindsToolOutsidePATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executable-bit semantics differ on Windows")
	}
	// A dir that stands in for ~/.local/bin, deliberately NOT on PATH.
	binDir := t.TempDir()
	tool := "sirsi-fake-agent"
	exe := filepath.Join(binDir, tool)
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake tool: %v", err)
	}

	// Truncated PATH that excludes binDir — the launchd condition.
	t.Setenv("PATH", "/usr/bin:/bin:/usr/sbin:/sbin")

	// Sanity: the OLD behavior (bare LookPath) must FAIL — proving the test is real.
	if _, err := exec.LookPath(tool); err == nil {
		t.Fatalf("precondition failed: %s unexpectedly on PATH", tool)
	}

	// Inject binDir as a candidate dir (stands in for ~/.local/bin).
	forceCandidateDirs = []string{binDir}
	t.Cleanup(func() { forceCandidateDirs = nil })

	got, ok := resolveBinary(tool)
	if !ok {
		t.Fatalf("resolveBinary(%q) = not found; want found via augmented dirs", tool)
	}
	if got != exe {
		t.Fatalf("resolveBinary(%q) = %q; want %q", tool, got, exe)
	}
}

// TestResolveBinary_PrefersPATH confirms the fast path still works when the tool
// IS on PATH (no behavior change for the normal case).
func TestResolveBinary_PrefersPATH(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "sirsi-fake-onpath")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	if _, ok := resolveBinary("sirsi-fake-onpath"); !ok {
		t.Fatal("resolveBinary should find a tool on PATH")
	}
}

func TestIsExecutableFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "x")
	_ = os.WriteFile(exe, []byte("x"), 0o755)
	if !isExecutableFile(exe) {
		t.Fatal("0755 file should be executable")
	}
	plain := filepath.Join(dir, "y")
	_ = os.WriteFile(plain, []byte("y"), 0o644)
	if isExecutableFile(plain) {
		t.Fatal("0644 file should NOT be executable")
	}
	if isExecutableFile(dir) {
		t.Fatal("a directory is not an executable file")
	}
}
