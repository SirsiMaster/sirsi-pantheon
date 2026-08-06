package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRegistry_Empty(t *testing.T) {
	tmp := t.TempDir()
	reg, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(reg.Agents) != 0 {
		t.Errorf("expected 0 agents for missing file, got %d", len(reg.Agents))
	}
}

func TestLoadRegistry_WithAgents(t *testing.T) {
	tmp := t.TempDir()
	os.WriteFile(filepath.Join(tmp, "agents.json"), []byte(`{
		"agents": {
			"claude-test": {
				"type": "claude",
				"command": ["claude", "--print"],
				"cwd": "/tmp/test"
			},
			"codex-test": {
				"type": "codex",
				"command": ["codex", "exec"],
				"cwd": "/tmp/test"
			}
		}
	}`), 0o644)

	reg, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(reg.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(reg.Agents))
	}
	if reg.Agents["claude-test"].ID != "claude-test" {
		t.Errorf("agent ID not injected from key")
	}
}

func TestLookup_Found(t *testing.T) {
	reg := &Registry{Agents: map[string]AgentConfig{
		"claude-test": {ID: "claude-test", Type: "claude", Command: []string{"claude"}, Cwd: "/tmp"},
	}}
	cfg, err := reg.Lookup("claude-test")
	if err != nil {
		t.Fatalf("Lookup() error: %v", err)
	}
	if cfg.Type != "claude" {
		t.Errorf("Type = %q, want claude", cfg.Type)
	}
}

func TestLookup_NotFound(t *testing.T) {
	reg := &Registry{Agents: map[string]AgentConfig{}}
	_, err := reg.Lookup("nonexistent")
	if err == nil {
		t.Error("expected error for unregistered agent")
	}
}

func TestValidate_Valid(t *testing.T) {
	cfg := AgentConfig{ID: "test", Type: "claude", Command: []string{"claude"}, Cwd: "/tmp"}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_MissingCommand(t *testing.T) {
	cfg := AgentConfig{ID: "test", Type: "claude", Cwd: "/tmp"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing command")
	}
}

func TestValidate_APICallWake(t *testing.T) {
	cfg := AgentConfig{
		ID:   "gemini-test",
		Type: "gemini",
		Wake: WakeConfig{Mechanism: WakeAPICall, Endpoint: "http://127.0.0.1/wake"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_MCPWake(t *testing.T) {
	cfg := AgentConfig{
		ID:   "cursor-test",
		Type: "ide-extension",
		Wake: WakeConfig{Mechanism: WakeMCPNotification, MCPServer: "sirsi"},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() error: %v", err)
	}
}

func TestValidate_EmptyType(t *testing.T) {
	cfg := AgentConfig{ID: "test", Command: []string{"cmd"}, Cwd: "/tmp"}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for empty type")
	}
}

func TestIsRegistered(t *testing.T) {
	reg := &Registry{Agents: map[string]AgentConfig{
		"claude-test": {ID: "claude-test"},
	}}
	if !reg.IsRegistered("claude-test") {
		t.Error("expected true for registered agent")
	}
	if reg.IsRegistered("unknown") {
		t.Error("expected false for unregistered agent")
	}
}

func TestSaveAndLoadRegistry(t *testing.T) {
	tmp := t.TempDir()
	reg := &Registry{Agents: map[string]AgentConfig{
		"test-agent": {ID: "test-agent", Type: "claude", Command: []string{"claude"}, Cwd: "/tmp"},
	}}
	if err := SaveRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveRegistry() error: %v", err)
	}
	loaded, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	if len(loaded.Agents) != 1 {
		t.Errorf("expected 1 agent after save/load, got %d", len(loaded.Agents))
	}
}

// TestLaunchAgentLabelRoundTrip is the regression test for the root cause of
// registry drift: WakeConfig had no LaunchAgentLabel field, so encoding/json
// silently discarded the JSON key on unmarshal, and every SaveRegistry call
// wrote the field away. One field addition closes the cycle permanently.
func TestLaunchAgentLabelRoundTrip(t *testing.T) {
	const label = "ai.sirsi.router.wake.claude-test"
	tmp := t.TempDir()
	reg := &Registry{Agents: map[string]AgentConfig{
		"claude-test": {
			ID: "claude-test", Type: "claude",
			Command: []string{"claude"}, Cwd: "/tmp",
			Wake: WakeConfig{Mechanism: WakeLaunchAgent, LaunchAgentLabel: label},
		},
	}}
	if err := SaveRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}
	loaded, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	got := loaded.Agents["claude-test"].Wake.LaunchAgentLabel
	if got != label {
		t.Errorf("launch_agent_label not preserved across save/load: got %q, want %q", got, label)
	}
}

// TestUnknownFieldPreservedRoundTrip is the regression test for the class of
// bug that erased claude-deck's consumer block: any field absent from
// AgentConfig's struct tags was silently dropped by json.MarshalIndent on
// SaveRegistry. This plants an unknown field and requires it to survive a
// complete disk round-trip (agents.json → LoadRegistry → SaveRegistry →
// LoadRegistry → raw parse).
func TestUnknownFieldPreservedRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	input := []byte(`{
		"agents": {
			"claude-deck": {
				"id": "claude-deck",
				"type": "claude",
				"command": ["claude", "--print"],
				"cwd": "/tmp",
				"wake": {"mechanism": "launchagent", "launch_agent_label": "ai.sirsi.router.wake.claude-deck"},
				"consumer": {
					"command": ["claude", "--print", "--permission-mode", "auto"],
					"prompt": "You are {{agent}}."
				}
			}
		}
	}`)

	if err := os.WriteFile(filepath.Join(tmp, "agents.json"), input, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	reg, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	if err = SaveRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	// Parse the saved file as raw JSON to check the consumer block survived.
	saved, err := os.ReadFile(filepath.Join(tmp, "agents.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal(saved, &out); err != nil {
		t.Fatalf("unmarshal saved: %v", err)
	}
	agents, _ := out["agents"].(map[string]any)
	deck, _ := agents["claude-deck"].(map[string]any)
	consumer, ok := deck["consumer"]
	if !ok {
		t.Fatal("consumer block was erased by SaveRegistry — unknown-field preservation is broken")
	}
	cm, _ := consumer.(map[string]any)
	if cm["prompt"] != "You are {{agent}}." {
		t.Errorf("consumer.prompt = %q, want %q", cm["prompt"], "You are {{agent}}.")
	}
}

// TestSaveRegistry_PreservesUnknownKeys is the regression test for the lossy
// round-trip defect: every SaveRegistry call via json.MarshalIndent(reg)
// silently dropped keys the Go struct has never modeled (e.g. "consumer" and
// nested "launch_agent_label" before it was added to WakeConfig). The lossless
// fix reads the existing file, deep-merges Go-known fields over the raw JSON
// map, and writes back — preserving every unknown key at every nesting depth.
func TestSaveRegistry_PreservesUnknownKeys(t *testing.T) {
	tmp := t.TempDir()

	// Seed agents.json with keys the Go struct does not model.
	seed := `{
  "agents": {
    "claude-deck": {
      "id": "claude-deck",
      "type": "claude",
      "command": ["claude"],
      "cwd": "/tmp/deck",
      "consumer": {
        "command": ["claude", "--print"],
        "prompt": "You are {{agent}}."
      },
      "wake": {
        "mechanism": "launchagent",
        "launch_agent_label": "ai.sirsi.router.wake.claude-deck"
      }
    }
  }
}`
	path := filepath.Join(tmp, "agents.json")
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load and re-save — this is the exact operation that previously lost keys.
	reg, loadErr := LoadRegistry(tmp)
	if loadErr != nil {
		t.Fatalf("LoadRegistry: %v", loadErr)
	}
	if saveErr := SaveRegistry(tmp, reg); saveErr != nil {
		t.Fatalf("SaveRegistry: %v", saveErr)
	}

	// Read the raw output and verify unknown keys survived.
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"consumer"`, `"prompt"`, `"launch_agent_label"`} {
		if !strings.Contains(s, want) {
			t.Errorf("SaveRegistry dropped key %s — lossless round-trip broken\n%s", want, s)
		}
	}
}

// TestSaveRegistry_ClearedKnownKeyAbsent is the regression guard for D1:
// a known Go struct field that is cleared to its zero value must be ABSENT
// from the output, not restored from the stale disk value (deepMergeJSON bug).
func TestSaveRegistry_ClearedKnownKeyAbsent(t *testing.T) {
	tmp := t.TempDir()
	seed := `{
  "agents": {
    "test-agent": {
      "id": "test-agent",
      "type": "claude",
      "command": ["claude"],
      "cwd": "/tmp",
      "wake": {
        "mechanism": "launchagent",
        "launch_agent_label": "ai.sirsi.old.stale.label"
      }
    }
  }
}`
	path := filepath.Join(tmp, "agents.json")
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, loadErr := LoadRegistry(tmp)
	if loadErr != nil {
		t.Fatalf("LoadRegistry: %v", loadErr)
	}
	// Simulate moving to mechanism:none — clears LaunchAgentLabel.
	agent := reg.Agents["test-agent"]
	agent.Wake = WakeConfig{Mechanism: WakeNone}
	reg.Agents["test-agent"] = agent

	if saveErr := SaveRegistry(tmp, reg); saveErr != nil {
		t.Fatalf("SaveRegistry: %v", saveErr)
	}

	saved, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	s := string(saved)
	if strings.Contains(s, "launch_agent_label") {
		t.Errorf("cleared launch_agent_label still present — stale disk value not erased:\n%s", s)
	}
	if strings.Contains(s, "stale.label") {
		t.Errorf("stale label value still present in output:\n%s", s)
	}
}

// TestSaveRegistry_CorruptExistingReturnsError is the regression guard for D2:
// a non-empty but unparseable existing file must return an error, not silently
// fall back to a fresh map and drop all unknown keys.
func TestSaveRegistry_CorruptExistingReturnsError(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "agents.json")
	if err := os.WriteFile(path, []byte(`{invalid json`), 0o644); err != nil {
		t.Fatal(err)
	}

	reg := &Registry{Agents: map[string]AgentConfig{
		"test-agent": {ID: "test-agent", Type: "claude", Command: []string{"claude"}, Cwd: "/tmp"},
	}}
	if err := SaveRegistry(tmp, reg); err == nil {
		t.Error("expected error for corrupted agents.json, got nil — silent data loss possible")
	}
}

func TestValidate_NoneWake(t *testing.T) {
	cfg := AgentConfig{
		ID: "codex-nexus", Type: "codex",
		Wake: WakeConfig{Mechanism: WakeNone},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("WakeNone must be valid (explicit opt-out), got: %v", err)
	}
}

// TestMarshalJSON_ExtraDoesNotShadowTypedField is the regression test for the
// latent defect in MarshalJSON: a typed field with omitempty present-but-empty
// on disk ("workstream":"") is absent from the instance-based knownJSON
// discovery, so it lands in extra. Without the "!ok" guard, the extra copy
// unconditionally overwrites the typed map entry — silently reverting any
// programmatic write to that field on every save.
func TestMarshalJSON_ExtraDoesNotShadowTypedField(t *testing.T) {
	// Simulate a hand-edited agents.json with an empty workstream field.
	// encoding/json with omitempty omits this field when marshaling, so the
	// discovery loop files it as "extra" rather than a known key.
	const raw = `{"id":"a","type":"claude","command":["x"],"cwd":"/tmp","workstream":""}`
	var cfg AgentConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Programmatic write to the typed field — this must survive marshal.
	cfg.Workstream = "deck"

	out, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	got := strings.Trim(string(m["workstream"]), `"`)
	if got != "deck" {
		t.Errorf("workstream = %q after programmatic write; want %q — extra shadowed typed field", got, "deck")
	}
}
