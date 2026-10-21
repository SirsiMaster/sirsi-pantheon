package completionproof

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func proofRepo(t *testing.T, name string) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	proof := filepath.Join(root, ".agents", "proofs", "item-1.json")
	if err := os.MkdirAll(filepath.Dir(proof), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".agents", "completion.contract.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proof, []byte(`{"repo":"`+name+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, proof
}

func validator(t *testing.T, root, proof string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "validator.py")
	body := fmt.Sprintf(`import os, sys
repo = os.path.realpath(sys.argv[sys.argv.index("--repo") + 1])
proof = os.path.realpath(sys.argv[sys.argv.index("--proof") + 1])
if repo != %q or proof != %q:
    print("wrong caller authority: repo=%%s proof=%%s" %% (repo, proof))
    sys.exit(9)
`, mustEval(t, root), mustEval(t, proof))
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return script
}

func mustEval(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestEnforceBindsProofToCallerWorktree(t *testing.T) {
	caller, proof := proofRepo(t, "application")
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", validator(t, caller, proof))

	if err := Enforce(caller, "item-1", filepath.Join(".agents", "proofs", "item-1.json"), false, false, "done"); err != nil {
		t.Fatalf("relative caller proof: %v", err)
	}
	if err := Enforce(caller, "item-1", proof, false, false, "done"); err != nil {
		t.Fatalf("absolute caller proof: %v", err)
	}
}

func TestEnforceRejectsProofFromAnotherRepository(t *testing.T) {
	caller, _ := proofRepo(t, "application")
	_, foreignProof := proofRepo(t, "other")
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", validator(t, caller, filepath.Join(caller, ".agents", "proofs", "item-1.json")))
	if err := Enforce(caller, "item-1", foreignProof, false, false, "done"); err == nil {
		t.Fatal("foreign proof unexpectedly accepted")
	}
	linkedProof := filepath.Join(caller, ".agents", "proofs", "foreign.json")
	if err := os.Symlink(foreignProof, linkedProof); err != nil {
		t.Fatal(err)
	}
	if err := Enforce(caller, "item-1", linkedProof, false, false, "done"); err == nil {
		t.Fatal("symlinked foreign proof unexpectedly accepted")
	}
}

func TestEnforceRefusesProofWhenCallerHasNoContract(t *testing.T) {
	root := t.TempDir()
	proof := filepath.Join(root, "proof.json")
	if err := os.WriteFile(proof, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Enforce(root, "item-1", proof, false, false, "done"); err == nil {
		t.Fatal("proof without caller contract unexpectedly accepted")
	}
}

func TestEnforceKeepsExplicitCoordinationOutcomes(t *testing.T) {
	caller, _ := proofRepo(t, "application")
	if err := Enforce(caller, "item-1", "", true, false, "blocked on upstream"); err != nil {
		t.Fatalf("blocked outcome: %v", err)
	}
	if err := Enforce(caller, "item-1", "", false, true, "received for review"); err != nil {
		t.Fatalf("acknowledgement outcome: %v", err)
	}
}
