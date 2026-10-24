// Package entryfixture supplies inert test inputs for the CLI and MCP close
// entry points. No product imports this package. Validation executes the
// byte-pinned portfolio helper, not a replacement policy implementation.
package entryfixture

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

//go:embed capture_and_forward.py
var validator []byte

//go:embed agent_completion_gate.py
var portfolioHelper []byte

const helperSHA256 = "a4953e61937d8bf03dfe9fbbcd6a0decb3fa44ad56fd4550fab0b0444cbf8ab9"

const Agent = "completion-entry-fixture"

type Case struct {
	Name, Refusal      string
	Status             interface{}
	OmitStatus         bool
	Blockers           []string
	VerificationStatus string
	MalformedJSON      bool
}

// The negative statuses are proof statuses, NOT --blocked/--ack coordination
// outcomes. Those intentionally retain different shipped semantics.
func Cases() []Case {
	return []Case{
		{Name: "completed", Status: "completed"},
		{Name: "draft", Status: "draft", Refusal: "completion requires status `completed`"},
		{Name: "blocked", Status: "blocked", Refusal: "completion requires status `completed`"},
		{Name: "ready-for-review", Status: "ready_for_review", Refusal: "completion requires status `completed`"},
		{Name: "missing-status", OmitStatus: true, Refusal: "completion requires status `completed`"},
		{Name: "malformed-status", Status: map[string]bool{"completed": true}, Refusal: "completion requires status `completed`"},
		{Name: "completed-with-blockers", Status: "completed", Blockers: []string{"unresolved"}, Refusal: "completion requires an empty blockers list"},
		{Name: "failed-verification", Status: "completed", VerificationStatus: "fail", Refusal: "final proof requires passing status"},
		{Name: "malformed-json", MalformedJSON: true, Refusal: "invalid completion proof on stdin"},
	}
}

type Fixture struct {
	Store              *routerstore.SQLiteStore
	ID, Proof, Receipt string
	Captured           string
}

func New(t *testing.T, tc Case) *Fixture {
	t.Helper()
	// HOME must be private before Resolve (including cutover-marker recovery),
	// SendGuarded's notification paths, or any caller discovery can run.
	t.Setenv("HOME", t.TempDir())
	for _, kv := range os.Environ() {
		key, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(key, "SIRSI_") || strings.HasPrefix(key, "GIT_") ||
			key == "CLAUDE_CODE_SESSION_ID" || key == "CODEX_THREAD_ID" || key == "CODEX_SESSION_ID" {
			t.Setenv(key, "")
			if err := os.Unsetenv(key); err != nil {
				t.Fatal(err)
			}
		}
	}
	root := t.TempDir()
	t.Chdir(root)
	git := exec.Command("git", "init", "-q", root)
	if out, err := git.CombinedOutput(); err != nil {
		t.Fatalf("owned fixture git init: %v: %s", err, out)
	}
	registry := filepath.Join(root, ".agents", "idea-router")
	if err := os.MkdirAll(registry, 0700); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(registry, "agents.json"), `{"agents":{"completion-entry-fixture":{"id":"completion-entry-fixture","type":"codex","repo":"/owned-fixture","workstream":"fixture","wake":{"mechanism":"none"}}}}`)
	contract := completeContract()
	writeJSON(t, filepath.Join(root, ".agents", "completion.contract.json"), contract)
	write(t, filepath.Join(root, "SCOPE.md"), "# Owned completion-entry fixture\nNo application, model, or live service is exercised.\n")
	write(t, filepath.Join(root, "evidence.txt"), "Inert evidence for the owned entry-point regression.\n")
	f := &Fixture{Proof: filepath.Join(root, "proof.json"), Receipt: filepath.Join(root, "validator-receipt.json")}
	if got := fmt.Sprintf("%x", sha256.Sum256(portfolioHelper)); got != helperSHA256 {
		t.Fatalf("retained portfolio helper changed: %s", got)
	}
	write(t, filepath.Join(root, "agent_completion_gate.py"), string(portfolioHelper))
	script := filepath.Join(root, "capture_and_forward.py")
	write(t, script, string(validator))
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", script)
	t.Setenv("SIRSI_COMPLETION_ENTRY_RECEIPT", f.Receipt)
	t.Setenv("SIRSI_ROUTER_REPO", root)
	t.Setenv("SIRSI_AGENT_ID", Agent)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	db := filepath.Join(root, "fixture.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	s, err := routerstore.OpenPath(db)
	if err != nil {
		t.Fatal(err)
	}
	f.Store = s
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	f.ID, _, err = s.SendGuarded(routerstore.SendReq{From: Agent, To: Agent, Title: "owned completion-entry fixture", Instructions: "No execution required."})
	if err != nil {
		t.Fatal(err)
	}
	document := completeProof(root, f.ID)
	document["status"] = tc.Status
	if tc.OmitStatus {
		delete(document, "status")
	}
	if tc.Blockers != nil {
		document["blockers"] = tc.Blockers
	}
	if tc.VerificationStatus != "" {
		document["verification"].([]map[string]interface{})[0]["status"] = tc.VerificationStatus
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if tc.MalformedJSON {
		raw = []byte(`{"status":`)
	}
	f.Captured = string(raw)
	write(t, f.Proof, f.Captured)
	return f
}

// AssertValidator proves the actual entry reached the completion-mode helper
// with exactly the captured bytes, independently of its error/success result.
func (f *Fixture) AssertValidator(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(f.Receipt)
	if err != nil {
		t.Fatalf("completion validator was not reached: %v", err)
	}
	var receipt struct {
		Mode, Captured string
	}
	if err := json.Unmarshal(data, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Mode != "validate-completion" || receipt.Captured != f.Captured {
		t.Fatalf("wrong completion protocol/captured bytes: %+v", receipt)
	}
}

// Adapted from Development/tools/test_completion_final_status.py's complete
// proof scaffold. Unlike its direct-function fixtures, these also carry a
// complete on-disk contract and nonempty canon, so the helper's real main()
// validates both contract and proof without mocked load_json/validate_contract.
func completeContract() map[string]interface{} {
	definitions := []map[string]interface{}{}
	for _, category := range []string{"product", "design", "technical", "operational", "narrative"} {
		definitions = append(definitions, map[string]interface{}{"category": category, "criteria": []string{"owned entry-point fixture"}})
	}
	return map[string]interface{}{
		"repo_name": "completion-entry-fixture", "release_target": "fixture-only", "classification": "internal-tool",
		"canon_documents":                []map[string]string{{"path": "SCOPE.md", "role": "scope"}},
		"done_definition":                definitions,
		"required_verification_commands": []map[string]string{{"name": "fixture", "command": "inert-fixture"}},
	}
}

func completeProof(root, id string) map[string]interface{} {
	closure := map[string]string{}
	for _, category := range []string{"product", "design", "technical", "operational", "narrative"} {
		closure[category] = "owned fixture: evidence.txt; not a live qualification"
	}
	return map[string]interface{}{
		"work_item_id": id, "agent_id": Agent, "repo": root,
		"status": "completed", "classification": "internal-tool",
		"canon_read":         []map[string]string{{"path": "SCOPE.md", "read_at": "2026-10-24T00:00:00Z"}},
		"requirements_trace": []map[string]interface{}{{"doc": "SCOPE.md", "section": "owned fixture", "claim": "entry wiring", "files_changed": []string{"SCOPE.md"}, "evidence": []string{"evidence.txt"}}},
		"closure_evidence":   closure,
		"verification":       []map[string]interface{}{{"name": "fixture", "command": "inert-fixture", "status": "pass", "evidence": "evidence.txt"}},
		"working_status":     map[string]string{"environment": "owned temp fixture", "how_verified": "entry-point regression", "observed_result": "fixture data, not live proof", "verified_at": "2026-10-24T00:00:00Z"},
		"handoffs":           []interface{}{}, "blockers": []string{},
	}
}

func writeJSON(t *testing.T, path string, value interface{}) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, string(raw))
}

func (f *Fixture) AssertUnchanged(t *testing.T, before routerstore.Item) {
	t.Helper()
	after, err := f.Store.Get(f.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("refused completion mutated item: before=%+v after=%+v", before, after)
	}
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}
