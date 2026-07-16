package router

import (
	"testing"
	"time"
)

// seedClaudeThread registers a live loop-monitor (claude surface) thread; its
// armed state is decided by the injected watcherAliveFn.
func seedClaudeThread(t *testing.T, root, agent, threadID string) {
	t.Helper()
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	reg.Threads[threadID] = &Thread{
		ThreadID:   threadID,
		AgentID:    agent,
		Surface:    "claude",
		PID:        70100,
		Status:     ThreadStatusActive,
		MachineID:  MachineID(),
		LastSeenAt: time.Now().UTC(),
	}
	if err := SaveThreadRegistry(root, reg); err != nil {
		t.Fatal(err)
	}
}

// TestAgentLoopDead is the router item 20260714-210359 contract: loop-dead is
// per-agent (open items + zero armed watchers), never per-session.
func TestAgentLoopDead(t *testing.T) {
	root := t.TempDir()
	// Three concurrent claude-home sessions (the CCD duplicate-record shape);
	// only thr-armed runs a live /loop.
	seedClaudeThread(t, root, "claude-home", "thr-armed")
	seedClaudeThread(t, root, "claude-home", "thr-idle-1")
	seedClaudeThread(t, root, "claude-home", "thr-idle-2")
	// claude-nexus: one session, loop dead.
	seedClaudeThread(t, root, "claude-nexus", "thr-nexus")

	old := watcherAliveFn
	watcherAliveFn = func(threadID string) bool { return threadID == "thr-armed" }
	defer func() { watcherAliveFn = old }()

	pending := map[string][]string{
		"claude-home":  {"item-1", "item-2"},
		"claude-nexus": {"item-3"},
		"claude-idle":  {},
	}

	cases := []struct {
		name  string
		agent string
		want  bool
	}{
		{"one armed watcher satisfies the agent — redundant sessions suppressed", "claude-home", false},
		{"open items + zero armed watchers → loop-dead", "claude-nexus", true},
		{"no open items → nothing stranding, no alarm", "claude-idle", false},
		{"unknown agent → no alarm", "claude-ghost", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AgentLoopDead(root, c.agent, pending); got != c.want {
				t.Errorf("AgentLoopDead(%s) = %v, want %v", c.agent, got, c.want)
			}
		})
	}
}
