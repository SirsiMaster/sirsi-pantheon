package main

// Pins the ADR-037 completion-proof close gate (restored 2026-07-22 after the
// Router v2 facade rewrite dropped it). The gate is the lever: these tests
// fail if a bare close ever slips past a contract-carrying repo again.

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnforceCompletionProof(t *testing.T) {
	contractRepo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(contractRepo, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contractRepo, ".agents", "completion.contract.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bareRepo := t.TempDir()

	cases := []struct {
		name    string
		repo    string
		proof   string
		blocked bool
		ack     bool
		result  string
		wantErr bool
	}{
		{"contract + bare close is rejected", contractRepo, "", false, false, "did it", true},
		{"contract + ack with result passes", contractRepo, "", false, true, "coordination ack", false},
		{"contract + blocked with result passes", contractRepo, "", true, false, "blocked on IAM", false},
		{"ack without result is rejected", contractRepo, "", false, true, "  ", true},
		{"blocked without result is rejected", contractRepo, "", true, false, "", true},
		{"no contract + bare close passes", bareRepo, "", false, false, "done", false},
		{"proof without contract is rejected", bareRepo, "p.json", false, false, "done", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := enforceCompletionProof(tc.repo, "item-1", tc.proof, tc.blocked, tc.ack, tc.result)
			if (err != nil) != tc.wantErr {
				t.Fatalf("got err=%v, wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestValidateCompletionProofUsesGateScript(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".agents", "completion.contract.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	proof := filepath.Join(repo, "proof.json")
	if err := os.WriteFile(proof, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	pass := filepath.Join(t.TempDir(), "pass.py")
	if err := os.WriteFile(pass, []byte("import sys; sys.exit(0)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fail := filepath.Join(t.TempDir(), "fail.py")
	if err := os.WriteFile(fail, []byte("import sys; print('missing evidence'); sys.exit(1)\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", pass)
	if err := enforceCompletionProof(repo, "item-1", proof, false, false, "done"); err != nil {
		t.Fatalf("passing validator: %v", err)
	}
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", fail)
	if err := enforceCompletionProof(repo, "item-1", proof, false, false, "done"); err == nil {
		t.Fatal("failing validator: want error, got nil")
	}
}
