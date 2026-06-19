package router

import "testing"

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
	got := computeStranded(root, pending)
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
