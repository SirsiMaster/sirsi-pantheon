package completionproof

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Data fixtures only. Policy always comes from the production embed.
func proofRepo(t *testing.T, name string) (string, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(filepath.Join(root, ".agents", "proofs"), 0o700); err != nil {
		t.Fatal(err)
	}
	root = mustEval(t, root)
	writeTest(t, filepath.Join(root, "SCOPE.md"), []byte("Owned inert test scope\n"))
	definitions := []map[string]any{}
	closure := map[string]string{}
	for _, category := range []string{"product", "design", "technical", "operational", "narrative"} {
		definitions = append(definitions, map[string]any{"category": category, "criteria": []string{"fixture"}})
		closure[category] = "inert evidence.txt"
	}
	contract := map[string]any{
		"repo_name": name, "release_target": "fixture", "classification": "internal-tool",
		"canon_documents":                []map[string]string{{"path": "SCOPE.md", "role": "scope"}},
		"done_definition":                definitions,
		"required_verification_commands": []map[string]string{{"name": "first", "command": "inert first"}, {"name": "second", "command": "inert second"}},
	}
	proof := map[string]any{
		"repo": root, "work_item_id": "item-1", "agent_id": "owned-test", "classification": "internal-tool", "status": "completed", "blockers": []string{},
		"canon_read":         []map[string]string{{"path": "SCOPE.md", "read_at": "2026-11-24T00:00:00Z"}},
		"requirements_trace": []map[string]any{{"doc": "SCOPE.md", "section": "fixture", "claim": "inert", "files_changed": []string{"SCOPE.md"}, "evidence": []string{"evidence.txt"}}},
		"closure_evidence":   closure,
		"verification":       []map[string]string{{"name": "first", "command": "inert first", "status": "pass", "evidence": "evidence.txt"}, {"name": "second", "command": "inert second", "status": "passed", "evidence": "evidence.txt"}},
		"working_status":     map[string]string{"environment": "owned temp", "how_verified": "inert fixture", "observed_result": "not a product qualification", "verified_at": "2026-11-24T00:00:00Z"},
		"handoffs":           []any{},
	}
	writeJSONTest(t, filepath.Join(root, ".agents", "completion.contract.json"), contract)
	path := filepath.Join(root, ".agents", "proofs", "item-1.json")
	writeJSONTest(t, path, proof)
	return root, path
}

func writeTest(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
func writeJSONTest(t *testing.T, path string, obj any) {
	t.Helper()
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, path, data)
}
func readTest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func mutateProof(t *testing.T, path string, change func(map[string]any)) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(readTest(t, path), &doc); err != nil {
		t.Fatal(err)
	}
	change(doc)
	writeJSONTest(t, path, doc)
}
func mustEval(t *testing.T, path string) string {
	t.Helper()
	actual, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return actual
}

func TestProductionCompletionProtocolAndStandaloneParity(t *testing.T) {
	cases := []struct {
		name   string
		change func(map[string]any)
		good   bool
	}{
		{"completed", func(map[string]any) {}, true},
		{"draft", func(p map[string]any) { p["status"] = "draft" }, false},
		{"blocked", func(p map[string]any) { p["status"] = "blocked"; p["blockers"] = []string{"pending"} }, false},
		{"ready", func(p map[string]any) { p["status"] = "ready_for_review" }, false},
		{"missing-status", func(p map[string]any) { delete(p, "status") }, false},
		{"malformed-status", func(p map[string]any) { p["status"] = map[string]bool{"completed": true} }, false},
		{"blockers", func(p map[string]any) { p["blockers"] = []string{"pending"} }, false},
		{"wrong-item", func(p map[string]any) { p["work_item_id"] = "item-2" }, false},
		{"padded-item", func(p map[string]any) { p["work_item_id"] = " item-1" }, false},
		{"classification-mismatch", func(p map[string]any) { p["classification"] = "commercial-product" }, false},
		{"required-omitted", func(p map[string]any) { p["verification"] = p["verification"].([]any)[:1] }, false},
		{"required-command-changed", func(p map[string]any) { p["verification"].([]any)[1].(map[string]any)["command"] = "different" }, false},
		{"required-name-changed", func(p map[string]any) { p["verification"].([]any)[1].(map[string]any)["name"] = "different" }, false},
		{"failed-verification", func(p map[string]any) { p["verification"].([]any)[1].(map[string]any)["status"] = "fail" }, false},
		{"duplicate-verification", func(p map[string]any) { v := p["verification"].([]any); p["verification"] = append(v, v[0]) }, false},
		{"missing-closure", func(p map[string]any) { delete(p["closure_evidence"].(map[string]any), "operational") }, false},
		{"missing-working-status", func(p map[string]any) { delete(p, "working_status") }, false},
		{"wrong-repo", func(p map[string]any) { p["repo"] = "/not-this-owned-fixture" }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root, proof := proofRepo(t, "application")
			mutateProof(t, proof, tc.change)
			err := Enforce(root, "item-1", proof, false, false, "done")
			if (err == nil) != tc.good {
				t.Fatalf("embedded result=%v; want accepted=%v", err, tc.good)
			}
			standalone := filepath.Join(root, "export.py")
			if err := ExportValidator(standalone); err != nil {
				t.Fatal(err)
			}
			if string(readTest(t, standalone)) != productionValidator {
				t.Fatal("export is not exact embedded source")
			}
			args := []string{"validate-completion", "--repo", root, "--proof", "/metadata-only-never-open", "--expected-item-id", "item-1"}
			if got := standaloneRun(t, root, standalone, readTest(t, proof), args...); got != tc.good {
				t.Fatalf("standalone accepted=%v; want %v", got, tc.good)
			}
		})
	}
}

func standaloneRun(t *testing.T, root, script string, input []byte, args ...string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), completionValidatorTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, completionPython, append([]string{"-I", "-S", script}, args...)...)
	cmd.Dir, cmd.Env, cmd.Stdin = root, completionValidatorEnv(os.Environ()), bytes.NewReader(input)
	cmd.WaitDelay = completionValidatorWaitDelay
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("standalone validator exceeded test deadline")
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("could not start validator: %v", err)
		}
	}
	return err == nil
}

func TestCapturedProofNotReopenedBothDirections(t *testing.T) {
	root, path := proofRepo(t, "application")
	_, relative, display, err := resolveProofCapture(root, path)
	if err != nil {
		t.Fatal(err)
	}
	good, err := captureCompletionProof(root, relative)
	if err != nil {
		t.Fatal(err)
	}
	mutateProof(t, path, func(p map[string]any) { p["status"] = "blocked"; p["blockers"] = []string{"pending"} })
	bad, err := captureCompletionProof(root, relative)
	if err != nil {
		t.Fatal(err)
	}
	if err := runCompletionValidator(root, "item-1", display, good); err != nil {
		t.Fatalf("pathname replacement overrode good capture: %v", err)
	}
	writeTest(t, path, good)
	if err := runCompletionValidator(root, "item-1", display, bad); err == nil {
		t.Fatal("good pathname overrode blocked capture")
	}
}

func TestProductionJSONAndContractRefusals(t *testing.T) {
	for _, raw := range []string{`{"status":`, `{"status":"completed","status":"completed"}`, `{"n":NaN}`, `{"n":1e999}`} {
		root, path := proofRepo(t, "application")
		writeTest(t, path, []byte(raw))
		if err := Enforce(root, "item-1", path, false, false, "done"); err == nil {
			t.Fatal("ambiguous/malformed JSON accepted")
		}
	}
	root, path := proofRepo(t, "application")
	contractPath := filepath.Join(root, ".agents", "completion.contract.json")
	var contract map[string]any
	if err := json.Unmarshal(readTest(t, contractPath), &contract); err != nil {
		t.Fatal(err)
	}
	commands := contract["required_verification_commands"].([]any)
	contract["required_verification_commands"] = append(commands, commands[0])
	writeJSONTest(t, contractPath, contract)
	if err := Enforce(root, "item-1", path, false, false, "done"); err == nil {
		t.Fatal("duplicate contract command identity accepted")
	}
}

func TestDiagnosticModesDoNotAuthorizeCompletion(t *testing.T) {
	root, path := proofRepo(t, "application")
	mutateProof(t, path, func(p map[string]any) { p["status"] = "blocked"; p["blockers"] = []string{"review"} })
	if err := Validate(root, path); err != nil {
		t.Fatalf("diagnostic blocked proof: %v", err)
	}
	if err := Enforce(root, "item-1", path, false, false, "done"); err == nil {
		t.Fatal("diagnostic acceptance authorized completion")
	}
	standalone := filepath.Join(root, "export.py")
	if err := ExportValidator(standalone); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"check-contract", "--repo", root},
		{"init-proof", "--repo", root, "--work-item", "new-item", "--agent-id", "owned-test"},
		{"validate", "--repo", root, "--proof", path},
	} {
		if !standaloneRun(t, root, standalone, nil, args...) {
			t.Fatalf("standalone mode refused: %v", args)
		}
	}
	initialized := filepath.Join(root, ".agents", "proofs", "new-item.json")
	before := readTest(t, initialized)
	if standaloneRun(t, root, standalone, nil, "init-proof", "--repo", root, "--work-item", "new-item") {
		t.Fatal("init overwrote existing proof")
	}
	if !bytes.Equal(before, readTest(t, initialized)) {
		t.Fatal("existing proof changed")
	}
	if standaloneRun(t, root, standalone, readTest(t, path), "validate-completion", "--repo", root, "--proof", path) {
		t.Fatal("completion without expected item accepted")
	}
}

func TestSubstitutedRootGitHelperCannotSupplyAuthority(t *testing.T) {
	root, path := proofRepo(t, "application")
	foreign, foreignProof := proofRepo(t, "foreign")
	for _, dir := range []string{root, foreign} {
		gitTest(t, dir, "init", "-q")
	}
	t.Chdir(root)
	fakebin := t.TempDir()
	marker := filepath.Join(fakebin, "executed")
	for _, name := range []string{"git", "python3"} {
		fake := filepath.Join(fakebin, name)
		writeTest(t, fake, []byte("#!/bin/sh\n: > '"+marker+"'\nexit 0\n"))
		if err := os.Chmod(fake, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	helper := filepath.Join(foreign, "forged.py")
	writeTest(t, helper, []byte("from pathlib import Path\nPath("+strconvQuote(marker)+").write_text('selected')\n"))
	t.Setenv("PATH", fakebin)
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", helper)
	t.Setenv("SIRSI_ROUTER_REPO", foreign)
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "core.worktree")
	t.Setenv("GIT_CONFIG_VALUE_0", foreign)
	if err := EnforceCurrentCaller(foreign, "item-1", path, false, false, "done"); err != nil {
		t.Fatalf("actual caller refused under substituted selectors: %v", err)
	}
	if err := EnforceCurrentCaller(foreign, "item-1", foreignProof, false, false, "done"); err == nil {
		t.Fatal("foreign Git/root substituted for actual caller")
	}
	mutateProof(t, path, func(p map[string]any) { p["status"] = "draft" })
	if err := EnforceCurrentCaller(foreign, "item-1", path, false, false, "done"); err == nil {
		t.Fatal("forged helper authorized invalid proof")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("substituted executable/helper ran: %v", err)
	}
}

func strconvQuote(s string) string { raw, _ := json.Marshal(s); return string(raw) }
func gitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(completionGit, args...)
	cmd.Dir, cmd.Env = dir, append(completionValidatorEnv(os.Environ()), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("owned Git fixture: %v: %s", err, out)
	}
}

func TestPythonEnvironmentCannotLoadCallerCode(t *testing.T) {
	root, path := proofRepo(t, "application")
	marker := filepath.Join(root, "startup-ran")
	writeTest(t, filepath.Join(root, "sitecustomize.py"), []byte("from pathlib import Path\nPath("+strconvQuote(marker)+").touch()\n"))
	t.Setenv("PYTHONPATH", root)
	t.Setenv("PYTHONHOME", root)
	t.Setenv("PYTHONSTARTUP", filepath.Join(root, "sitecustomize.py"))
	if err := Enforce(root, "item-1", path, false, false, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("caller Python startup ran: %v", err)
	}
	for _, env := range completionValidatorEnv([]string{"PYTHONPATH=x", "DYLD_INSERT_LIBRARIES=x", "LD_PRELOAD=x", "XCRUN_TOOLCHAIN=x", "DEVELOPER_DIR=x", "SDKROOT=x", "TOOLCHAINS=x", "GIT_DIR=x", "SIRSI_ROUTER_REPO=x", "HOME=/owned"}) {
		if env != "HOME=/owned" {
			t.Fatalf("unsafe environment retained: %q", env)
		}
	}
}

func TestLinkedCallerAndLocalGitRedirect(t *testing.T) {
	root, proof := proofRepo(t, "application")
	gitTest(t, root, "init", "-q")
	gitTest(t, root, "-c", "user.name=owned-test", "-c", "user.email=owned@example.invalid", "commit", "--allow-empty", "-qm", "owned fixture")
	linked := filepath.Join(mustEval(t, t.TempDir()), "relocated-linked-worktree")
	gitTest(t, root, "worktree", "add", "-q", "-b", "owned-linked", linked, "HEAD")
	if err := os.MkdirAll(filepath.Join(linked, ".agents", "proofs"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"SCOPE.md", ".agents/completion.contract.json", ".agents/proofs/item-1.json"} {
		writeTest(t, filepath.Join(linked, rel), readTest(t, filepath.Join(root, rel)))
	}
	linkedProof := filepath.Join(linked, ".agents", "proofs", "item-1.json")
	mutateProof(t, linkedProof, func(p map[string]any) { p["repo"] = linked })
	marker, err := os.Lstat(filepath.Join(linked, ".git"))
	if err != nil || !marker.Mode().IsRegular() {
		t.Fatalf("linked fixture must use an actual regular gitfile: %v, %v", marker, err)
	}
	t.Chdir(linked)
	if err := EnforceCurrentCaller("/no-canonical-install-here", "item-1", ".agents/proofs/item-1.json", false, false, "done"); err != nil {
		t.Fatalf("relocated linked caller refused: %v", err)
	}
	linkedNested := filepath.Join(linked, "nested", "caller")
	if err := os.MkdirAll(linkedNested, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(linkedNested)
	if err := EnforceCurrentCaller("/no-canonical-install-here", "item-1", linkedProof, false, false, "done"); err != nil {
		t.Fatalf("nested linked caller refused: %v", err)
	}
	t.Chdir(root)
	if err := EnforceCurrentCaller("/no-canonical-install-here", "item-1", proof, false, false, "done"); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested", "caller")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	if err := EnforceCurrentCaller("/no-canonical-install-here", "item-1", proof, false, false, "done"); err != nil {
		t.Fatalf("nested directory-backed caller refused: %v", err)
	}
	gitTest(t, root, "config", "core.worktree", linked)
	if err := EnforceCurrentCaller(root, "item-1", linkedProof, false, false, "done"); err == nil {
		t.Fatal("local Git redirect supplied foreign caller authority")
	}
}

func TestCallerRefusesSymlinkGitBoundaryMarker(t *testing.T) {
	root, _ := proofRepo(t, "application")
	gitTest(t, root, "init", "-q")
	marker := filepath.Join(root, ".git")
	retained := filepath.Join(root, "retained-git-directory")
	if err := os.Rename(marker, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(retained, marker); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "nested-caller")
	if err := os.Mkdir(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	if err := EnforceCurrentCaller(root, "item-1", "", false, false, "done"); err == nil || !strings.Contains(err.Error(), "invalid physical Git boundary marker") {
		t.Fatalf("symlink marker did not fail at physical boundary: %v", err)
	}
}

func TestNearestGitBoundaryDoesNotSkipInvalidChildMarker(t *testing.T) {
	root := mustEval(t, t.TempDir())
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, ".git"), filepath.Join(child, ".git")); err != nil {
		t.Fatal(err)
	}
	if boundary, err := nearestGitBoundary(child); err == nil || boundary != "" {
		t.Fatalf("invalid child marker fell through to ancestor: boundary=%q error=%v", boundary, err)
	}
}

func TestBareCompletionRefusesUpwardGitWorktreeRedirect(t *testing.T) {
	root, _ := proofRepo(t, "application")
	gitTest(t, root, "init", "-q")
	ancestor := filepath.Dir(root)
	if _, err := os.Stat(filepath.Join(ancestor, ".agents", "completion.contract.json")); !os.IsNotExist(err) {
		t.Fatalf("owned ancestor must have no completion contract: %v", err)
	}
	subdir := filepath.Join(root, "actual-caller-subdir")
	if err := os.Mkdir(subdir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(subdir)
	// First establish that this same bare request discovers the actual child
	// repository's contract without the upward core.worktree redirect.
	if err := EnforceCurrentCaller(root, "item-1", "", false, false, "done"); err == nil || !strings.Contains(err.Error(), "completion proof required") {
		t.Fatalf("actual child contract did not require proof before redirect: %v", err)
	}
	gitTest(t, root, "config", "core.worktree", ancestor)
	// Confirm the fixture really makes fixed Git report an ancestor that still
	// contains cwd. A sibling redirect would exercise a different existing guard.
	cmd := exec.Command(completionGit, "-C", subdir, "rev-parse", "--show-toplevel")
	cmd.Dir = subdir
	cmd.Env = append(completionValidatorEnv(os.Environ()), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	output, err := cmd.Output()
	if err != nil || strings.TrimSuffix(string(output), "\n") != ancestor {
		t.Fatalf("fixture did not report the contract-free ancestor: root=%q err=%v", output, err)
	}
	// Empty proof is essential: accepting a contract-free ancestor must not turn
	// a child repository's proof-required implementation close into a bare close.
	if err := EnforceCurrentCaller(root, "item-1", "", false, false, "done"); err == nil {
		t.Fatal("upward Git worktree redirect bypassed actual child repository completion contract on bare close")
	}
}

func TestCompletionContainmentLimitsAndCoordination(t *testing.T) {
	root, path := proofRepo(t, "application")
	_, foreign := proofRepo(t, "foreign")
	if err := Enforce(root, "item-1", foreign, false, false, "done"); err == nil {
		t.Fatal("foreign proof accepted")
	}
	writeTest(t, path, make([]byte, maxCompletionProofBytes+1))
	if err := Enforce(root, "item-1", path, false, false, "done"); err == nil {
		t.Fatal("oversized proof accepted")
	}
	if err := Enforce(root, "item-1", "", false, false, "done"); err == nil {
		t.Fatal("bare contract completion accepted")
	}
	for _, outcome := range []struct{ blocked, ack bool }{{true, false}, {false, true}} {
		if err := Enforce("", "item-1", "", outcome.blocked, outcome.ack, "owned coordination"); err != nil {
			t.Fatal(err)
		}
		if err := Enforce("", "item-1", "", outcome.blocked, outcome.ack, " "); err == nil {
			t.Fatal("coordination missing result accepted")
		}
	}
	t.Chdir(t.TempDir())
	if err := EnforceCurrentCaller("forged", "item-1", "", false, true, "coordination"); err != nil {
		t.Fatal(err)
	}
	if err := EnforceCurrentCaller("forged", "item-1", "", false, false, "done"); err == nil {
		t.Fatal("non-Git completion accepted")
	}
}

func TestValidatorDeadlineAndOutputBounds(t *testing.T) {
	root, path := proofRepo(t, "application")
	if err := runValidatorCommand(root, []string{"validate-completion", "--repo", root, "--proof", path, "--expected-item-id", "item-1"}, readTest(t, path), 0*time.Second); err == nil {
		t.Fatal("expired validator deadline accepted")
	}
	var output limitedOutput
	if n, err := output.Write(make([]byte, 1<<20)); err != nil || n != 1<<20 || !output.exceeded || len(output.data) != 16<<10 {
		t.Fatal("output retention is not bounded")
	}
	if n, err := output.Write([]byte("tail")); err != nil || n != 4 || len(output.data) != 16<<10 {
		t.Fatal("output bound did not remain stable")
	}
	if strings.Contains(ValidatorSHA256(), "\n") || len(ValidatorSHA256()) != 64 {
		t.Fatal("invalid source identity")
	}
}
