package router

import (
	"strings"
	"testing"
)

// launchagent is a first-class wake mechanism, NOT the "unsupported" default —
// Validate must accept it (with the same command+cwd requirements as cli-spawn,
// which it wraps). This is the true source of the board's old false
// "unsupported wake mechanism launchagent" label.
func TestValidateAcceptsLaunchAgent(t *testing.T) {
	cfg := AgentConfig{
		ID:      "claude-test",
		Type:    "claude",
		Command: []string{"claude", "--print"},
		Cwd:     t.TempDir(),
		Wake:    WakeConfig{Mechanism: WakeLaunchAgent},
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("launchagent config should validate, got: %v", err)
	}

	// Missing command still errors, and the message names the mechanism.
	bad := AgentConfig{ID: "x", Type: "claude", Cwd: t.TempDir(), Wake: WakeConfig{Mechanism: WakeLaunchAgent}}
	err := bad.Validate()
	if err == nil || !strings.Contains(err.Error(), WakeLaunchAgent) {
		t.Errorf("missing-command launchagent should error naming the mechanism, got: %v", err)
	}
}

// agentWakeReady for a launchagent agent must mirror ProbeWakeReadiness: READY
// only when the plist is actually installed. A configured-but-not-installed
// launchagent is genuinely not wakeable yet, so it reports NOT ready with the
// actionable fix — never a green false-positive (the review's blocking finding).
func TestAgentWakeReadyLaunchAgentNotInstalled(t *testing.T) {
	cfg := AgentConfig{
		ID:      "ghost-agent-not-installed-xyz",
		Type:    "claude",
		Command: []string{"claude", "--print"},
		Cwd:     t.TempDir(),
		Wake:    WakeConfig{Mechanism: WakeLaunchAgent},
	}
	// Sanity: no plist for this fabricated id.
	if WakeLaunchAgentInstalled(cfg.ID) {
		t.Skip("unexpected: a wake plist exists for the fabricated test id")
	}
	ready, detail := agentWakeReady(cfg)
	if ready {
		t.Errorf("not-installed launchagent must NOT be ready (masks a real alarm); detail=%q", detail)
	}
	if !strings.Contains(detail, "wake-install") {
		t.Errorf("detail should point at the fix `sirsi router wake-install`, got %q", detail)
	}
}
