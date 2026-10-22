package completionproof

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	if err := os.WriteFile(proof, []byte(`{"repo":"`+name+`","status":"completed","blockers":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, proof
}

func validator(t *testing.T, root, proof string) string {
	t.Helper()
	script := filepath.Join(t.TempDir(), "validator.py")
	expected, err := os.ReadFile(proof)
	if err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`import json, os, sys
mode = sys.argv[1]
repo = os.path.realpath(sys.argv[sys.argv.index("--repo") + 1])
proof = sys.argv[sys.argv.index("--proof") + 1]
if repo != %q:
    print("wrong caller authority: repo=%%s proof=%%s" %% (repo, proof))
    sys.exit(9)
if mode == "validate":
    if os.path.realpath(proof) != %q:
        print("wrong proof pathname: %%s" %% proof)
        sys.exit(10)
elif mode == "validate-completion":
    if os.path.realpath(proof) != %q:
        print("wrong proof metadata: %%s" %% proof)
        sys.exit(11)
    if os.environ.get("SIRSI_TEST_SUBSTITUTE_PROOF") == "1":
        with open(proof, "wb") as changed:
            changed.write(b'{"status":"blocked","blockers":["substituted"]}')
    captured = sys.stdin.buffer.read()
    if os.environ.get("SIRSI_TEST_SUBSTITUTE_PROOF") == "1":
        with open(proof, "wb") as restored:
            restored.write((%q).encode("utf-8"))
    if captured != (%q).encode("utf-8"):
        print("validator did not receive the captured proof bytes")
        sys.exit(12)
    document = json.loads(captured)
    if document.get("status") != "completed" or document.get("blockers") not in (None, []):
        print("completion requires completed proof without blockers")
        sys.exit(13)
else:
    print("unexpected validator mode: %%s" %% mode)
    sys.exit(14)
`, mustEval(t, root), mustEval(t, proof), mustEval(t, proof), expected, expected)
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return script
}

func TestEnforceCompletionValidatorReceivesOneCapturedSnapshot(t *testing.T) {
	caller, proof := proofRepo(t, "application")
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", validator(t, caller, proof))
	t.Setenv("SIRSI_TEST_SUBSTITUTE_PROOF", "1")

	if err := Enforce(caller, "item-1", proof, false, false, "done"); err != nil {
		t.Fatalf("completion snapshot rejected: %v", err)
	}
	contents, err := os.ReadFile(proof)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != `{"repo":"application","status":"completed","blockers":[]}` {
		t.Fatalf("substitution-back did not restore original proof: %s", contents)
	}
}

func TestEnforceRejectsNonFinalProofBeforeRouterClose(t *testing.T) {
	caller, proof := proofRepo(t, "application")
	if err := os.WriteFile(proof, []byte(`{"repo":"application","status":"blocked","blockers":["unresolved"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", validator(t, caller, proof))

	err := Enforce(caller, "item-1", proof, false, false, "done")
	if err == nil {
		t.Fatal("blocked proof unexpectedly accepted for implementation completion")
	}
	if !strings.Contains(err.Error(), "completion requires completed proof without blockers") {
		t.Fatalf("blocked proof failed for the wrong reason: %v", err)
	}
}

func TestStandaloneValidateKeepsLegacyWorkInProgressMode(t *testing.T) {
	caller, proof := proofRepo(t, "application")
	if err := os.WriteFile(proof, []byte(`{"repo":"application","status":"blocked","blockers":["review pending"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", validator(t, caller, proof))

	if err := Validate(caller, proof); err != nil {
		t.Fatalf("standalone work-in-progress validation changed unexpectedly: %v", err)
	}
}

func TestEnforceFailsClosedWhenValidatorLacksCompletionMode(t *testing.T) {
	caller, proof := proofRepo(t, "application")
	legacyOnly := filepath.Join(t.TempDir(), "legacy-only.py")
	if err := os.WriteFile(legacyOnly, []byte(`import sys
if sys.argv[1] == "validate":
    raise SystemExit(0)
print("unknown command", file=sys.stderr)
raise SystemExit(64)
`), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", legacyOnly)

	if err := Enforce(caller, "item-1", proof, false, false, "done"); err == nil {
		t.Fatal("completion unexpectedly fell back to the legacy validator")
	}
}

func TestEnforceBoundsCapturedProof(t *testing.T) {
	caller, proof := proofRepo(t, "application")
	if err := os.WriteFile(proof, make([]byte, maxCompletionProofBytes+1), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Enforce(caller, "item-1", proof, false, false, "done"); err == nil {
		t.Fatal("oversized completion proof unexpectedly accepted")
	}
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

func TestEnforceCurrentCallerKeepsCoordinationAvailableOutsideGit(t *testing.T) {
	clearInheritedGitEnvironment(t)
	t.Chdir(t.TempDir())
	if err := EnforceCurrentCaller("", "item-1", "", false, true, "received for review"); err != nil {
		t.Fatalf("non-Git acknowledgement: %v", err)
	}
	if err := EnforceCurrentCaller("", "item-1", "", true, false, "waiting for upstream"); err != nil {
		t.Fatalf("non-Git blocker: %v", err)
	}
	if err := EnforceCurrentCaller("", "item-1", "", false, false, "done"); err == nil {
		t.Fatal("non-Git implementation completion unexpectedly accepted")
	}
}

// The caller can be a linked worktree below a .worktrees directory. Its proof
// must still be validated by the portfolio tool next to the canonical router,
// not by a made-up sibling of the caller worktree.
func TestEnforceUsesCanonicalPortfolioValidatorForLinkedCallerWithoutOverride(t *testing.T) {
	clearInheritedGitEnvironment(t)
	development := t.TempDir()
	canonical := filepath.Join(development, "sirsi-pantheon")
	tools := filepath.Join(development, "tools")
	appMain := filepath.Join(development, "compasspoint-client-reporting")
	appWorktree := filepath.Join(appMain, ".worktrees", "reporting")
	for _, dir := range []string{canonical, tools, appMain} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(canonical, ".agents", "idea-router"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(canonical, "README"), []byte("router\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, canonical, "init", "-q")

	git(t, appMain, "init", "-q")
	git(t, appMain, "config", "user.email", "tests@example.invalid")
	git(t, appMain, "config", "user.name", "tests")
	if err := os.WriteFile(filepath.Join(appMain, "README"), []byte("application\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, appMain, "add", "README")
	git(t, appMain, "commit", "-qm", "initial")
	git(t, appMain, "worktree", "add", "-q", "-b", "reporting", appWorktree, "HEAD")

	proof := filepath.Join(appWorktree, ".agents", "proofs", "item-1.json")
	if err := os.MkdirAll(filepath.Dir(proof), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appWorktree, ".agents", "completion.contract.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proof, []byte(`{"repo":"application"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	canonicalProof, err := filepath.EvalSymlinks(proof)
	if err != nil {
		t.Fatal(err)
	}
	canonicalCaller, err := filepath.EvalSymlinks(appWorktree)
	if err != nil {
		t.Fatal(err)
	}
	validatorPath := filepath.Join(tools, "agent_completion_gate.py")
	validatorBody := fmt.Sprintf(`import os, sys
repo = os.path.realpath(sys.argv[sys.argv.index("--repo") + 1])
proof = os.path.realpath(sys.argv[sys.argv.index("--proof") + 1])
if repo != %q or proof != %q:
    print("wrong caller authority: repo=%%s proof=%%s" %% (repo, proof))
    sys.exit(9)
`, canonicalCaller, canonicalProof)
	if err := os.WriteFile(validatorPath, []byte(validatorBody), 0o700); err != nil {
		t.Fatal(err)
	}

	// No SIRSI_COMPLETION_GATE_SCRIPT override: the handler's canonical router
	// root selects the Development/tools validator even though the current
	// directory is the linked application worktree.
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", "")
	t.Chdir(appWorktree)
	if err := EnforceCurrentCaller(canonical, "item-1", filepath.Join(".agents", "proofs", "item-1.json"), false, false, "done"); err != nil {
		t.Fatalf("canonical validator with linked caller: %v", err)
	}
}

// Git hooks export repository variables for their own commands. Tests that
// deliberately construct a non-Git caller or an independent nested worktree
// must not inherit those variables as accidental caller authority.
func clearInheritedGitEnvironment(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"GIT_DIR",
		"GIT_WORK_TREE",
		"GIT_COMMON_DIR",
		"GIT_INDEX_FILE",
		"GIT_OBJECT_DIRECTORY",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES",
		"GIT_PREFIX",
	} {
		value, wasSet := os.LookupEnv(key)
		key, value, wasSet := key, value, wasSet
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		t.Cleanup(func() {
			if wasSet {
				_ = os.Setenv(key, value)
				return
			}
			_ = os.Unsetenv(key)
		})
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
