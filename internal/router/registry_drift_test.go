package router

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRegistryRoundTripPreservesLaunchAgentLabel guards the root cause of the
// 2026-07-31 registry drift landmine: WakeConfig lacked a LaunchAgentLabel
// field, so SaveRegistry silently dropped launch_agent_label from every
// launchagent entry on every round-trip. The fix adds the field + auto-fills
// on load. This test fails if the regression returns.
func TestRegistryRoundTripPreservesLaunchAgentLabel(t *testing.T) {
	tmp := t.TempDir()
	const label = "ai.sirsi.router.wake.claude-test"
	original := `{
  "agents": {
    "claude-test": {
      "type": "claude",
      "command": ["claude", "--print"],
      "cwd": "/tmp",
      "wake": {
        "mechanism": "launchagent",
        "launch_agent_label": "` + label + `"
      }
    }
  }
}`
	if err := os.WriteFile(filepath.Join(tmp, "agents.json"), []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	cfg := reg.Agents["claude-test"]
	if cfg.Wake.LaunchAgentLabel != label {
		t.Fatalf("after load: LaunchAgentLabel=%q, want %q", cfg.Wake.LaunchAgentLabel, label)
	}

	// Round-trip through SaveRegistry — the label must survive.
	if err := SaveRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveRegistry: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(tmp, "agents.json"))
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	agents := raw["agents"].(map[string]any)
	claude := agents["claude-test"].(map[string]any)
	wake := claude["wake"].(map[string]any)
	got, _ := wake["launch_agent_label"].(string)
	if got != label {
		t.Fatalf("after save/reload: launch_agent_label=%q, want %q — struct field may be missing", got, label)
	}
}

// TestLoadRegistryAutoFillsLaunchAgentLabel ensures that a launchagent entry
// without an explicit launch_agent_label gets it auto-filled on load so that
// SaveRegistry never drops it on the next write.
func TestLoadRegistryAutoFillsLaunchAgentLabel(t *testing.T) {
	tmp := t.TempDir()
	// Deliberately omit launch_agent_label — simulates a drifted registry.
	raw := `{
  "agents": {
    "claude-io": {
      "type": "claude",
      "command": ["claude", "--print"],
      "cwd": "/tmp",
      "wake": { "mechanism": "launchagent" }
    }
  }
}`
	os.WriteFile(filepath.Join(tmp, "agents.json"), []byte(raw), 0o644)

	reg, err := LoadRegistry(tmp)
	if err != nil {
		t.Fatal(err)
	}
	want := WakeLaunchAgentLabel("claude-io")
	if got := reg.Agents["claude-io"].Wake.LaunchAgentLabel; got != want {
		t.Fatalf("auto-fill: LaunchAgentLabel=%q, want %q", got, want)
	}
}

// TestValidateAcceptsWakeNone ensures mechanism:none does not fail Validate.
// Agents that explicitly opt out of waking (codex-* that run only interactively)
// must be expressible in the registry without triggering an "unsupported mechanism" error.
func TestValidateAcceptsWakeNone(t *testing.T) {
	cfg := AgentConfig{
		ID:      "codex-test",
		Type:    "codex",
		Command: []string{"codex", "exec"},
		Cwd:     "/tmp",
		Wake:    WakeConfig{Mechanism: WakeNone},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("mechanism:none should validate, got: %v", err)
	}
}

// TestWakeFieldDiff exercises the helper used by RegistryDrift.
func TestWakeFieldDiff(t *testing.T) {
	tests := []struct {
		name      string
		main      agentWakeSnapshot
		disk      agentWakeSnapshot
		wantEmpty bool
	}{
		{
			name:      "identical",
			main:      agentWakeSnapshot{mechanism: "launchagent", launchAgentLabel: "ai.sirsi.router.wake.x"},
			disk:      agentWakeSnapshot{mechanism: "launchagent", launchAgentLabel: "ai.sirsi.router.wake.x"},
			wantEmpty: true,
		},
		{
			name:      "label missing from disk",
			main:      agentWakeSnapshot{mechanism: "launchagent", launchAgentLabel: "ai.sirsi.router.wake.x"},
			disk:      agentWakeSnapshot{mechanism: "launchagent", launchAgentLabel: ""},
			wantEmpty: false,
		},
		{
			name:      "mechanism changed",
			main:      agentWakeSnapshot{mechanism: "launchagent"},
			disk:      agentWakeSnapshot{mechanism: "none"},
			wantEmpty: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff := wakeFieldDiff(tt.main, tt.disk)
			if tt.wantEmpty && diff != "" {
				t.Errorf("expected no diff, got %q", diff)
			}
			if !tt.wantEmpty && diff == "" {
				t.Errorf("expected a diff, got empty string")
			}
		})
	}
}
