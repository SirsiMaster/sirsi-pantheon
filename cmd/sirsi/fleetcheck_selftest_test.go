package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The fleet-check script carries its own negative and positive controls
// (--self-test): a Mac with no deploy key and a parked item must read RED naming
// both causes, and a held-only outbox must read green. Running it here keeps those
// controls in CI, so the check cannot drift narrower than the claim it makes (A35).
func TestFleetCheckSelfTest(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl not available")
	}
	script := filepath.Join("..", "..", "scripts", "fleet-check.sh")
	out, err := exec.Command("bash", script, "--self-test").CombinedOutput()
	if err != nil {
		t.Fatalf("fleet-check --self-test failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "self-test passed") {
		t.Fatalf("self-test did not report passing:\n%s", out)
	}
}
