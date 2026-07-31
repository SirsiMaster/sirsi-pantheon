package router

import (
	"encoding/json"
	"os"
	"path/filepath"
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

func TestValidate_NoneWake(t *testing.T) {
	cfg := AgentConfig{
		ID: "codex-nexus", Type: "codex",
		Wake: WakeConfig{Mechanism: WakeNone},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("WakeNone must be valid (explicit opt-out), got: %v", err)
	}
}
