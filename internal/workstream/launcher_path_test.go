package workstream

import (
	"os"
	"path/filepath"
	"testing"
)

// writeFakeBin creates an executable stub named `name` under dir and returns its path.
func writeFakeBin(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

// TestLookPathFindsBinariesOutsideLaunchdPATH is the core P14 regression: a CLI
// installed under ~/.local/bin must resolve even when PATH is the sanitized set
// launchd hands GUI agents (which excludes ~/.local/bin and Homebrew).
func TestLookPathFindsBinariesOutsideLaunchdPATH(t *testing.T) {
	home := t.TempDir()
	want := writeFakeBin(t, filepath.Join(home, ".local", "bin"), "claude")

	t.Setenv("HOME", home)
	// Simulate launchd's truncated PATH: no ~/.local/bin, no Homebrew.
	t.Setenv("PATH", "/usr/bin:/bin")

	got, err := lookPath("claude")
	if err != nil {
		t.Fatalf("lookPath(claude) returned error with binary in ~/.local/bin: %v", err)
	}
	if got != want {
		t.Errorf("lookPath(claude) = %q, want %q", got, want)
	}
}

// TestLookPathFindsCodexInHomeBin covers the second tool named in the P14 report
// and a different fallback dir (~/bin).
func TestLookPathFindsCodexInHomeBin(t *testing.T) {
	home := t.TempDir()
	want := writeFakeBin(t, filepath.Join(home, "bin"), "codex")

	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")

	got, err := lookPath("codex")
	if err != nil {
		t.Fatalf("lookPath(codex) returned error with binary in ~/bin: %v", err)
	}
	if got != want {
		t.Errorf("lookPath(codex) = %q, want %q", got, want)
	}
}

// TestLookPathPrefersInheritedPATH confirms the fallback does not override a tool
// that the inherited PATH already resolves.
func TestLookPathPrefersInheritedPATH(t *testing.T) {
	home := t.TempDir()
	// A "real" PATH dir with the binary, plus a decoy in ~/.local/bin.
	pathDir := t.TempDir()
	onPath := writeFakeBin(t, pathDir, "gemini")
	writeFakeBin(t, filepath.Join(home, ".local", "bin"), "gemini")

	t.Setenv("HOME", home)
	t.Setenv("PATH", pathDir+":/usr/bin:/bin")

	got, err := lookPath("gemini")
	if err != nil {
		t.Fatalf("lookPath(gemini) error: %v", err)
	}
	if got != onPath {
		t.Errorf("lookPath(gemini) = %q, want PATH hit %q", got, onPath)
	}
}

// TestLookPathMissingBinary verifies a genuinely absent tool still errors (so the
// fallback never produces a false "installed").
func TestLookPathMissingBinary(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("PATH", "/usr/bin:/bin")

	if _, err := lookPath("definitely-not-a-real-cli-xyz"); err == nil {
		t.Error("expected error for a binary that exists nowhere")
	}
}

// TestLookPathIgnoresNonExecutable ensures a non-executable file of the right name
// is not mistaken for an installed tool.
func TestLookPathIgnoresNonExecutable(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	// Mode 0644 — present but not executable.
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")

	if _, err := lookPath("claude"); err == nil {
		t.Error("expected error: a non-executable file should not count as installed")
	}
}

// TestClaudeInstalledViaAugmentedPath exercises the actual Launcher.Installed path
// (the surface the menubar calls) end-to-end through the augmented resolver.
func TestClaudeInstalledViaAugmentedPath(t *testing.T) {
	home := t.TempDir()
	writeFakeBin(t, filepath.Join(home, ".local", "bin"), "claude")
	t.Setenv("HOME", home)
	t.Setenv("PATH", "/usr/bin:/bin")

	if !(ClaudeLauncher{}).Installed(&mockPlatform{home: home, shell: "/bin/zsh"}) {
		t.Error("ClaudeLauncher.Installed = false despite claude in ~/.local/bin (launchd-PATH regression)")
	}
}
