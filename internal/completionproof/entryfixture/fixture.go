// Package entryfixture supplies inert test inputs for the CLI and MCP close
// entry points. No product imports this package. Policy is the product embed,
// never this package's historical Python copies or caller helper overrides.
package entryfixture

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routercfg"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerstore"
)

const Agent = "completion-entry-fixture"

type Case struct {
	Name, Refusal                                             string
	Status                                                    interface{}
	OmitStatus                                                bool
	Blockers                                                  []string
	VerificationStatus                                        string
	MalformedJSON                                             bool
	WrongItem, MissingCommand, WrongCommand, DuplicateCommand bool
}

// The negative statuses are proof statuses, NOT --blocked/--ack coordination
// outcomes. Those intentionally retain different shipped semantics.
func Cases() []Case {
	cases := []Case{
		{Name: "completed", Status: "completed"},
		{Name: "draft", Status: "draft", Refusal: "completion requires status `completed`"},
		{Name: "blocked", Status: "blocked", Refusal: "completion requires status `completed`"},
		{Name: "ready-for-review", Status: "ready_for_review", Refusal: "completion requires status `completed`"},
		{Name: "missing-status", OmitStatus: true, Refusal: "completion requires status `completed`"},
		{Name: "malformed-status", Status: map[string]bool{"completed": true}, Refusal: "completion requires status `completed`"},
		{Name: "completed-with-blockers", Status: "completed", Blockers: []string{"unresolved"}, Refusal: "completion requires an empty blockers list"},
		{Name: "failed-verification", Status: "completed", VerificationStatus: "fail", Refusal: "final proof requires passing status"},
		{Name: "malformed-json", MalformedJSON: true, Refusal: "invalid completion proof on stdin"},
		{Name: "wrong-item", Status: "completed", WrongItem: true, Refusal: "refuse"},
		{Name: "missing-required-command", Status: "completed", MissingCommand: true, Refusal: "refuse"},
		{Name: "wrong-required-command", Status: "completed", WrongCommand: true, Refusal: "refuse"},
		{Name: "duplicate-command", Status: "completed", DuplicateCommand: true, Refusal: "refuse"},
	}
	// Product errors intentionally do not echo proof-derived diagnostics.
	for i := range cases {
		if cases[i].Refusal != "" {
			cases[i].Refusal = "completion proof validation failed"
		}
	}
	return cases
}

type Fixture struct {
	Store              routerstore.Store
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
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	git := exec.Command("/usr/bin/git", "init", "-q", root)
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
	// A selectable-helper regression would either create this marker or falsely
	// accept negative proofs. The true policy cannot be supplied by the fixture.
	script := filepath.Join(root, "forged-helper.py")
	markerJSON, err := json.Marshal(f.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	write(t, script, "from pathlib import Path\nPath("+string(markerJSON)+").write_text('forged helper selected')\n")
	t.Setenv("SIRSI_COMPLETION_GATE_SCRIPT", script)
	t.Setenv("SIRSI_ROUTER_REPO", root)
	t.Setenv("SIRSI_AGENT_ID", Agent)
	t.Setenv(routercfg.StoreWakeEnv, "1")
	db := filepath.Join(root, "fixture.db")
	t.Setenv("SIRSI_ROUTER_DB", db)
	s, err := routerstore.Resolve()
	if err != nil {
		t.Fatal(err)
	}
	f.Store = s
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	// This helper is tracked non-_test.go source, so it follows the same
	// ADR-062 resolver boundary. Its explicit private DB is a deliberate local
	// fixture, never a fallback to the operator's canonical ledger or service.
	if _, ok := s.(*routerstore.SQLiteStore); !ok {
		t.Fatalf("fixture resolver selected a non-SQLite backend: %T", s)
	}
	if info, err := os.Lstat(db); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("fixture resolver did not create the owned regular DB %q: %v", db, err)
	}
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
	if tc.WrongItem {
		document["work_item_id"] = f.ID + "-different"
	}
	if tc.MissingCommand {
		document["verification"] = document["verification"].([]map[string]interface{})[:1]
	}
	if tc.WrongCommand {
		document["verification"].([]map[string]interface{})[1]["command"] = "different-command"
	}
	if tc.DuplicateCommand {
		checks := document["verification"].([]map[string]interface{})
		document["verification"] = append(checks, checks[0])
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

// AssertValidator retains the existing entry-test API, but now proves only
// nonselection of the forged external helper and preservation of proof bytes.
// Actual status/lease assertions remain in the callers. Captured-stdin causality
// is independently exercised against the production helper in gate_test.go;
// there is deliberately no injectable policy/capture wrapper in product code.
func (f *Fixture) AssertValidator(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(f.Receipt); !os.IsNotExist(err) {
		t.Fatalf("forged helper selected or unexpected marker error: %v", err)
	}
	data, err := os.ReadFile(f.Proof)
	if err != nil || string(data) != f.Captured {
		t.Fatalf("entry changed fixture proof: %v", err)
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
		"required_verification_commands": []map[string]string{{"name": "fixture", "command": "inert-fixture"}, {"name": "second", "command": "inert-second"}},
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
		"verification":       []map[string]interface{}{{"name": "fixture", "command": "inert-fixture", "status": "pass", "evidence": "evidence.txt"}, {"name": "second", "command": "inert-second", "status": "pass", "evidence": "evidence.txt"}},
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
