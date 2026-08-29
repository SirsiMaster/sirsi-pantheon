package completiongate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateAcceptsFinalProof(t *testing.T) {
	repo := fixtureRepo(t, "completed", "passed")
	if err := Validate(repo, "proof.json"); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejectsFailedFinalVerification(t *testing.T) {
	repo := fixtureRepo(t, "completed", "failed")
	if err := Validate(repo, "proof.json"); err == nil {
		t.Fatal("final proof with failed verification was accepted")
	}
}

func TestValidateRejectsWrongProofRepo(t *testing.T) {
	repo := fixtureRepo(t, "draft", "not-run")
	path := filepath.Join(repo, "proof.json")
	if err := os.WriteFile(path, []byte(`{"work_item_id":"item","agent_id":"agent","repo":"/wrong","status":"draft","classification":"platform-foundation","canon_read":[{"path":"README.md","read_at":"now"}],"requirements_trace":[{"doc":"README.md","section":"s","claim":"c","files_changed":["f"],"evidence":["e"]}],"closure_evidence":{"product":"x","design":"x","technical":"x","operational":"x","narrative":"x"},"verification":[{"name":"test","command":"go test","status":"not-run","evidence":"e"}],"working_status":{"environment":"x","how_verified":"x","observed_result":"x","verified_at":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(repo, path); err == nil {
		t.Fatal("wrong proof repo was accepted")
	}
}

func TestValidateAcceptsResolvedRepoIdentity(t *testing.T) {
	repo := fixtureRepo(t, "completed", "passed")
	alias := filepath.Join(t.TempDir(), "checkout")
	if err := os.Symlink(repo, alias); err != nil {
		t.Fatal(err)
	}
	if err := Validate(alias, "proof.json"); err != nil {
		t.Fatalf("resolved repository identity: %v", err)
	}
}

func TestValidateRejectsMalformedHandoff(t *testing.T) {
	repo := fixtureRepo(t, "draft", "not-run")
	path := filepath.Join(repo, "proof.json")
	proof, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	broken := string(proof[:len(proof)-1]) + `,"handoffs":[{"from":"agent"}]}`
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Validate(repo, path); err == nil {
		t.Fatal("malformed handoff was accepted")
	}
}

func fixtureRepo(t *testing.T, status, verification string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	contract := `{"repo_name":"fixture","release_target":"fixture","classification":"platform-foundation","canon_documents":[{"path":"README.md","role":"requirements"}],"done_definition":[{"category":"product","criteria":["x"]},{"category":"design","criteria":["x"]},{"category":"technical","criteria":["x"]},{"category":"operational","criteria":["x"]},{"category":"narrative","criteria":["x"]}],"required_verification_commands":[{"name":"test","command":"go test"}]}`
	if err := os.WriteFile(filepath.Join(repo, ".agents", "completion.contract.json"), []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	proof := `{"work_item_id":"item","agent_id":"agent","repo":"` + repo + `","status":"` + status + `","classification":"platform-foundation","canon_read":[{"path":"README.md","read_at":"now"}],"requirements_trace":[{"doc":"README.md","section":"s","claim":"c","files_changed":["f"],"evidence":["e"]}],"closure_evidence":{"product":"x","design":"x","technical":"x","operational":"x","narrative":"x"},"verification":[{"name":"test","command":"go test","status":"` + verification + `","evidence":"e"}],"working_status":{"environment":"x","how_verified":"x","observed_result":"x","verified_at":"x"},"handoffs":[],"blockers":[]}`
	if err := os.WriteFile(filepath.Join(repo, "proof.json"), []byte(proof), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}
