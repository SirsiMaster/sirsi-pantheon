package setup

import (
	"runtime"
	"testing"
)

func TestDependenciesShape(t *testing.T) {
	deps := Dependencies()
	if len(deps) == 0 {
		t.Fatal("Dependencies() returned empty list")
	}

	var gitFound, requiredCount int
	for _, d := range deps {
		if d.Name == "" {
			t.Error("dependency with empty Name")
		}
		if d.InstallCmd == "" {
			t.Errorf("dependency %q has no InstallCmd", d.Name)
		}
		if d.Required {
			requiredCount++
		}
		if d.Name == "git" {
			gitFound++
			if !d.Required {
				t.Error("git must be a required dependency")
			}
		}
		// Regression guard: the npm thoth shims were false-negatives because
		// Thoth ships inside the sirsi binary. They must never reappear here.
		if d.Name == "thoth-init" || d.Name == "thoth-sync" || d.Name == "thoth-compact" {
			t.Errorf("npm thoth shim %q must not be listed; thoth ships with sirsi", d.Name)
		}
	}
	if gitFound != 1 {
		t.Errorf("expected exactly one git dependency, got %d", gitFound)
	}
	if requiredCount == 0 {
		t.Error("expected at least one required dependency")
	}
}

func TestNeedsFDA(t *testing.T) {
	cases := map[string]bool{
		"scan":    true,
		"ghosts":  true,
		"clean":   true,
		"analyze": true,
		"status":  false,
		"version": false,
		"setup":   false,
		"":        false,
		"thread":  false,
	}
	for cmd, want := range cases {
		if got := NeedsFDA(cmd); got != want {
			t.Errorf("NeedsFDA(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestFullDiskAccessGrantedNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin: FDA state depends on host grant")
	}
	if !FullDiskAccessGranted() {
		t.Error("non-darwin FullDiskAccessGranted() must be true (not applicable)")
	}
}

func TestOpenFullDiskAccessPaneNonDarwin(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("darwin: would launch System Settings")
	}
	if err := OpenFullDiskAccessPane(); err != nil {
		t.Errorf("non-darwin OpenFullDiskAccessPane() must be a no-op, got %v", err)
	}
}

func TestBinaryPathNonEmpty(t *testing.T) {
	if BinaryPath() == "" {
		t.Error("BinaryPath() must never be empty")
	}
}

func TestInstalledGitConsistency(t *testing.T) {
	// git is near-universal on dev hosts and in CI; if present, the version
	// string (when returned) must be non-empty-or-empty but never panic.
	d := Dependency{Name: "git", InstallCmd: "noop"}
	ok, ver := d.Installed()
	if ok && len(ver) >= 80 {
		t.Errorf("version string should be truncated/omitted when long, got %q", ver)
	}
}
