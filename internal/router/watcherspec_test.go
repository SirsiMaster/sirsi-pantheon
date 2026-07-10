package router

import (
	"strings"
	"testing"
)

func TestWatcherFor_Claude(t *testing.T) {
	s := WatcherFor("claude", "claude-pantheon", "thr-abc123")
	if s.Type != "loop-monitor" {
		t.Errorf("claude Type = %q, want loop-monitor", s.Type)
	}
	if !s.WatchesInbox {
		t.Error("claude must watch the inbox")
	}
	if s.Resident {
		t.Error("claude is not a resident surface")
	}
	if s.HeartbeatIntervalS != 60 {
		t.Errorf("claude heartbeat = %d, want 60", s.HeartbeatIntervalS)
	}
	// claude-deck's correction: the idempotency signature MUST key on the
	// thread_id, never the shared DIR= loop body, never TaskList.
	if !strings.Contains(s.ArmInstruction, "pgrep -f thr-abc123") {
		t.Errorf("arm_instruction must key on thread_id signature; got: %s", s.ArmInstruction)
	}
	if !strings.Contains(s.ArmInstruction, "NOT TaskList") {
		t.Error("arm_instruction must forbid keying on TaskList")
	}
}

// TestWatcherFor_StoreWakeCutover: with the ADR-036 cutover flag on, the Claude
// /loop and the surface-loop arm instructions must direct wake to `sirsi router
// wait` (the store FIFO) and explicitly stop watching the items/ directory —
// while keeping the thread-id idempotency signature.
func TestWatcherFor_StoreWakeCutover(t *testing.T) {
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "1")

	claude := WatcherFor("claude", "claude-pantheon", "thr-abc123")
	if !strings.Contains(claude.ArmInstruction, "sirsi router wait claude-pantheon") {
		t.Errorf("cutover claude arm must call `router wait`; got: %s", claude.ArmInstruction)
	}
	if strings.Contains(claude.ArmInstruction, "watching items/") {
		t.Errorf("cutover claude arm must NOT watch items/; got: %s", claude.ArmInstruction)
	}
	if !strings.Contains(claude.ArmInstruction, "pgrep -f thr-abc123") {
		t.Errorf("cutover claude arm must keep the thread-id signature; got: %s", claude.ArmInstruction)
	}

	gemma := WatcherFor("gemma", "gemma-4", "thr-xyz")
	if !strings.Contains(gemma.ArmInstruction, "sirsi router wait gemma-4") {
		t.Errorf("cutover surface-loop arm must call `router wait`; got: %s", gemma.ArmInstruction)
	}
}

// TestWatcherFor_LegacyDefaultWatchesItems: with the flag OFF (default), the arm
// instruction stays the legacy items/-directory watch — a binary shipped with
// the flag off behaves exactly as before.
func TestWatcherFor_LegacyDefaultWatchesItems(t *testing.T) {
	// Ensure the env is unset for this test regardless of ambient state.
	t.Setenv("SIRSI_ROUTER_STORE_WAKE", "0")
	claude := WatcherFor("claude", "claude-pantheon", "thr-abc123")
	if !strings.Contains(claude.ArmInstruction, "watching items/") {
		t.Errorf("legacy claude arm must watch items/; got: %s", claude.ArmInstruction)
	}
	if strings.Contains(claude.ArmInstruction, "router wait") {
		t.Errorf("legacy claude arm must NOT reference router wait; got: %s", claude.ArmInstruction)
	}
}

func TestWatcherFor_MenubarResident(t *testing.T) {
	s := WatcherFor("menubar", "sirsi-menubar", "thr-x")
	if s.Type != "native-runloop" {
		t.Errorf("menubar Type = %q, want native-runloop", s.Type)
	}
	if s.WatchesInbox {
		t.Error("resident menubar must NOT be an inbox worker (preserves ADR-020)")
	}
	if !s.Resident {
		t.Error("menubar must be marked resident")
	}
	if s.HeartbeatIntervalS < 60 {
		t.Errorf("menubar heartbeat = %d, want >=60", s.HeartbeatIntervalS)
	}
}

func TestWatcherFor_Idempotent(t *testing.T) {
	a := WatcherFor("claude", "claude-pantheon", "thr-1")
	b := WatcherFor("claude", "claude-pantheon", "thr-1")
	if a != b {
		t.Error("same (surface, agent, thread) must yield an identical spec")
	}
}

func TestWatcherFor_UnknownFallsBackToPullLoop(t *testing.T) {
	s := WatcherFor("nonsense", "x", "thr-1")
	if s.Type != "pull-loop" {
		t.Errorf("unknown surface Type = %q, want pull-loop fallback", s.Type)
	}
	if !s.WatchesInbox {
		t.Error("pull-loop fallback must watch the inbox")
	}
	if strings.Contains(s.ArmInstruction, "router daemon") || strings.Contains(s.Mechanism, "router daemon") {
		t.Fatalf("watcher spec must not prescribe removed router daemon verb: %+v", s)
	}
}

func TestWatcherFor_ResidentSurfacesNotInboxWorkers(t *testing.T) {
	for _, sfc := range []string{"menubar", "tui", "vscode", "jetbrains", "cursor", "macapp"} {
		s := WatcherFor(sfc, "x", "thr-1")
		if !s.Resident || s.WatchesInbox {
			t.Errorf("surface %q: want resident && !watches_inbox, got resident=%v watches=%v", sfc, s.Resident, s.WatchesInbox)
		}
	}
}

func TestWatcherFor_HeadlessSurfacesDoNotPrescribeDaemon(t *testing.T) {
	for _, sfc := range []string{"gemini", "gemma", "qwen", "mcp", "api", "webhook", "worker"} {
		s := WatcherFor(sfc, "x", "thr-1")
		if strings.Contains(s.ArmInstruction, "router daemon") || strings.Contains(s.Mechanism, "router daemon") {
			t.Fatalf("surface %q must not prescribe removed router daemon verb: %+v", sfc, s)
		}
		if !s.WatchesInbox {
			t.Errorf("surface %q must still watch/pull its inbox", sfc)
		}
	}
}
