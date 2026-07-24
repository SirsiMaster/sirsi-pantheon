package router

import (
	"errors"
	"testing"
)

// errNotLoaded is what a launchctl checker returns for an unloaded label.
var errNotLoaded = errors.New("not loaded")

// TestComputeStranded_FlagsUnarmedBacklog: agents with open items but no armed
// thread (loop-dead, or no thread at all) are surfaced as stranded; armed agents
// (live loop, or app-heartbeat) are not.
func TestComputeStranded_FlagsUnarmedBacklog(t *testing.T) {
	root := t.TempDir()
	armed, err := RegisterThread(root, &Thread{AgentID: "claude-armed", Surface: "claude", PID: 5001, StartTime: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterThread(root, &Thread{AgentID: "codex-app", Surface: "codex", PID: 5002, StartTime: "s"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RegisterThread(root, &Thread{AgentID: "claude-dead", Surface: "claude", PID: 5003, StartTime: "s"}); err != nil {
		t.Fatal(err)
	}

	// Only claude-armed has a live loop.
	old := watcherAliveFn
	watcherAliveFn = func(id string) bool { return id == armed.ThreadID }
	defer func() { watcherAliveFn = old }()

	pending := map[string][]string{
		"claude-armed": {"i1"},       // loop-alive → not stranded
		"codex-app":    {"i2"},       // app-heartbeat fresh → not stranded
		"claude-dead":  {"i3", "i4"}, // loop dead → STRANDED (2)
		"claude-gone":  {"i5"},       // no thread at all → STRANDED (1)
	}
	got := computeStranded(root, pending, nil)
	if len(got) != 2 {
		t.Fatalf("got %d stranded, want 2: %+v", len(got), got)
	}
	if got[0].AgentID != "claude-dead" || got[0].OpenItems != 2 {
		t.Errorf("first stranded = %+v, want claude-dead/2 (highest backlog first)", got[0])
	}
	if got[1].AgentID != "claude-gone" {
		t.Errorf("second stranded = %+v, want claude-gone", got[1])
	}

	if !AgentArmed(root, "claude-armed") || !AgentArmed(root, "codex-app") {
		t.Error("live-loop + app-heartbeat agents must report armed")
	}
	if AgentArmed(root, "claude-dead") || AgentArmed(root, "claude-gone") {
		t.Error("loop-dead + no-thread agents must report unarmed")
	}
}

// TestComputeStranded_CreditsWakeJobAndSkipsUser pins the two false-positive
// fixes (owner item 172430 / claude-home item 160154): an agent whose ONLY
// consumer is a launchd `ai.sirsi.router.wake.<agent>` job (no registry thread)
// is NOT stranded — crediting only the registry falsely flagged claude-pantheon
// while its wake job was live (PID 80806). And `user` (the owner queue) is never
// stranded, since it has no session watcher to "arm".
func TestComputeStranded_CreditsWakeJobAndSkipsUser(t *testing.T) {
	root := t.TempDir()
	pending := map[string][]string{
		"claude-wakeonly": {"i1"},       // no thread, but wake job live → NOT stranded
		"claude-nobody":   {"i2", "i3"}, // no thread, no wake job → STRANDED (2)
		"user":            {"i4"},       // owner queue → never stranded
	}
	// liveWake mirrors what liveWakeAgents(launchctlCheck) would produce.
	liveWake := map[string]bool{"claude-wakeonly": true}

	got := computeStranded(root, pending, liveWake)
	if len(got) != 1 {
		t.Fatalf("got %d stranded, want exactly 1 (claude-nobody): %+v", len(got), got)
	}
	if got[0].AgentID != "claude-nobody" || got[0].OpenItems != 2 {
		t.Errorf("stranded = %+v, want claude-nobody/2", got[0])
	}
}

// TestLiveWakeAgents confirms the launchd wake-label union: an agent is credited
// iff `launchctl list ai.sirsi.router.wake.<agent>` exits 0, and a nil checker
// (tests without launchctl) yields an empty set (registry-only classification).
func TestLiveWakeAgents(t *testing.T) {
	check := func(args ...string) error {
		if len(args) == 2 && args[0] == "list" && args[1] == "ai.sirsi.router.wake.claude-up" {
			return nil // loaded
		}
		return errNotLoaded
	}
	live := liveWakeAgents([]string{"claude-up", "claude-down"}, check)
	if !live["claude-up"] || live["claude-down"] {
		t.Errorf("live = %+v, want only claude-up", live)
	}
	if got := liveWakeAgents([]string{"claude-up"}, nil); len(got) != 0 {
		t.Errorf("nil checker must yield empty set, got %+v", got)
	}
}

// TestAgentHasLiveThread guards the wake-install leak fix (owner finding
// 2026-07-10): a live thread means a running session, so arming a background
// wake LaunchAgent on top would spawn duplicates. A freshly registered thread is
// live; a suspended one is not (so a fully-suspended agent may still be armed to
// wake a headless worker for its backlog); an absent agent is not live.
func TestAgentHasLiveThread(t *testing.T) {
	root := t.TempDir()

	if _, err := RegisterThread(root, &Thread{AgentID: "claude-live", Surface: "claude", PID: 6001, StartTime: "s"}); err != nil {
		t.Fatal(err)
	}
	if !AgentHasLiveThread(root, "claude-live") {
		t.Error("a freshly registered active thread must count as live")
	}
	if AgentHasLiveThread(root, "claude-absent") {
		t.Error("an agent with no threads must not be live")
	}

	susp, err := RegisterThread(root, &Thread{AgentID: "claude-susp", Surface: "claude", PID: 6002, StartTime: "s"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := SuspendThread(root, susp.ThreadID, &SuspendPayload{}); err != nil {
		t.Fatal(err)
	}
	if AgentHasLiveThread(root, "claude-susp") {
		t.Error("an agent with only a suspended thread must not be live")
	}
}
