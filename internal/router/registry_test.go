package router

import (
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

// TestSaveLoadRoundTripPreservesLaunchAgentLabel guards the root cause of the
// three registry drift incidents: WakeConfig lacked the LaunchAgentLabel field,
// so any SaveRegistry call silently dropped launch_agent_label from agents.json.
func TestSaveLoadRoundTripPreservesLaunchAgentLabel(t *testing.T) {
	tmp := t.TempDir()
	want := "ai.sirsi.router.wake.claude-pantheon"
	reg := &Registry{Agents: map[string]AgentConfig{
		"claude-pantheon": {
			ID: "claude-pantheon", Type: "claude",
			Command: []string{"claude"}, Cwd: "/tmp",
			Wake: WakeConfig{Mechanism: WakeLaunchAgent, LaunchAgentLabel: want},
		},
	}}
	if err := SaveRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveRegistry() error: %v", err)
	}
	loaded, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	got := loaded.Agents["claude-pantheon"].Wake.LaunchAgentLabel
	if got != want {
		t.Errorf("launch_agent_label after round-trip = %q, want %q (field was dropped by SaveRegistry)", got, want)
	}
}

// TestLoadRegistryAutoFillsLaunchAgentLabel verifies that LoadRegistry fills
// LaunchAgentLabel for launchagent entries that lack it in the JSON — so the
// first save after this fix self-heals registries written before the field existed.
func TestLoadRegistryAutoFillsLaunchAgentLabel(t *testing.T) {
	tmp := t.TempDir()
	// JSON without launch_agent_label — as every registry written before this fix.
	os.WriteFile(filepath.Join(tmp, "agents.json"), []byte(`{
		"agents": {
			"claude-io": {
				"type": "claude",
				"command": ["claude"],
				"cwd": "/tmp",
				"wake": {"mechanism": "launchagent"}
			}
		}
	}`), 0o644)

	reg, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry() error: %v", err)
	}
	got := reg.Agents["claude-io"].Wake.LaunchAgentLabel
	want := WakeLaunchAgentLabel("claude-io")
	if got != want {
		t.Errorf("auto-fill: LaunchAgentLabel = %q, want %q", got, want)
	}
}

// TestValidateAcceptsWakeNone verifies that mechanism:none passes Validate,
// fixing ValidateAll() failures for every codex-* agent that opts out of waking.
func TestValidateAcceptsWakeNone(t *testing.T) {
	cfg := AgentConfig{
		ID:   "codex-nexus",
		Type: "codex",
		Wake: WakeConfig{Mechanism: WakeNone},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate() for mechanism:none error: %v", err)
	}
}
