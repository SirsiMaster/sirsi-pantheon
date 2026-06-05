package setup

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestArmMaatGate verifies the gate arms inside a source clone that ships
// .githooks/pre-push, is idempotent, and writes core.hooksPath=.githooks.
func TestArmMaatGate(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	// Pantheon source clones carry .agents/idea-router (FindRepoRoot anchor) and
	// the shipped gate.
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".githooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".githooks", "pre-push"), []byte("#!/bin/bash\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// ArmMaatGate resolves the repo root from the working directory.
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	res := ArmMaatGate()
	if res.Status != StatusOK {
		t.Fatalf("ArmMaatGate() = %v (%s), want StatusOK", res.Status, res.Message)
	}
	if res.Surface != SurfaceMaatGate {
		t.Errorf("surface = %q, want %q", res.Surface, SurfaceMaatGate)
	}
	out, err := exec.Command("git", "-C", dir, "config", "--get", "core.hooksPath").Output()
	if err != nil || string(out) != ".githooks\n" {
		t.Errorf("core.hooksPath = %q (err %v), want .githooks", out, err)
	}

	// Idempotent: a second arm is a clean no-op success.
	if res2 := ArmMaatGate(); res2.Status != StatusOK {
		t.Errorf("second ArmMaatGate() = %v, want StatusOK (idempotent)", res2.Status)
	}
}

// TestArmMaatGateSkipsNonClone verifies a directory without the shipped gate
// skips cleanly rather than arming or failing.
func TestArmMaatGateSkipsNonClone(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	defer os.Chdir(wd)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if res := ArmMaatGate(); res.Status != StatusSkipped {
		t.Errorf("ArmMaatGate() without .githooks = %v, want StatusSkipped", res.Status)
	}
}
