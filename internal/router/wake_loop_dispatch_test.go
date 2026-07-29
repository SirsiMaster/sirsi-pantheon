package router

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// markerScript writes a script that appends one line per invocation, so a test
// can count how many times the loop actually spawned a consumer.
func markerScript(t *testing.T, marker string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "consumer.sh")
	body := "#!/bin/sh\necho fired >> " + marker + "\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write consumer script: %v", err)
	}
	return path
}

func markerCount(t *testing.T, marker string) int {
	t.Helper()
	b, err := os.ReadFile(marker)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	return len(strings.Fields(string(b)))
}

// The loop must dispatch a consumer when its inbox is non-empty — the defect
// this guards is a loop that heartbeats forever beside items it never drains,
// which ALSO marks the agent `armed` in WakePass and so suppresses the adapter
// wake that would otherwise rescue it.
//
// And it must dispatch exactly ONCE while the inbox stays non-empty. `depth > 0`
// is a level, not an edge: it holds for the entire time an agent works its
// inbox, so a per-tick dispatch forks a new agent session every interval on top
// of the one already draining (the PR #199 fork-storm class). The consumer here
// deliberately does NOT drain the inbox, which is precisely the condition that
// made the original loop storm.
func TestWakeLoopDispatchesOnceWhileInboxStaysNonEmpty(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "fired.txt")
	script := markerScript(t, marker)

	root := wakeTestRoot(t, AgentConfig{
		ID:      "worker-agent",
		Type:    "worker", // not interactive — constraint 3 does not apply
		Command: []string{script},
	})
	sendItem(t, root, "worker-agent", "work that is never drained")

	// ~6 ticks. A per-tick dispatch would leave ~6 marks; the latch leaves 1.
	ctx, cancel := context.WithTimeout(context.Background(), 320*time.Millisecond)
	defer cancel()
	if err := RunWakeLoop(ctx, root, "worker-agent", 50*time.Millisecond); err != nil {
		t.Fatalf("RunWakeLoop: %v", err)
	}

	switch n := markerCount(t, marker); {
	case n == 0:
		t.Error("wake loop never dispatched a consumer — it heartbeat beside a non-empty " +
			"inbox, which is the watch-only loop this fix removes")
	case n > 1:
		t.Errorf("wake loop dispatched %d consumers while the inbox stayed non-empty — "+
			"dispatch is level-triggered, not edge-triggered (fork-storm)", n)
	}
}

// Constraint 3: a bare `claude` is a REPL. Detaching one per non-empty inbox
// would fork orphaned TTY-less sessions, and even a working one is a NEW
// conversation rather than a nudge to the running one. The loop must stay
// watch-only for it — and must NOT be fooled into dispatching by the fact that
// ProbeWakeReadiness calls such agents wakeable via the launchagent adapter.
func TestInteractiveAgentIsNeverDispatched(t *testing.T) {
	cmd, why := headlessConsumerCommand(AgentConfig{
		ID: "claude-deck", Type: "claude", Command: []string{"claude"},
	})
	if cmd != nil {
		t.Errorf("interactive agent returned a dispatch command %v — constraint 3 violated", cmd)
	}
	if !strings.Contains(why, "interactive") {
		t.Errorf("refusal reason %q does not explain the interactive gate", why)
	}

	// A --print invocation IS headless, so it is a genuine consumer.
	cmd, why = headlessConsumerCommand(AgentConfig{
		ID: "claude-nexus", Type: "claude", Command: []string{"sh", "--print"},
	})
	if cmd == nil {
		t.Errorf("headless --print agent was refused: %s", why)
	}
}

// A loop with nothing to dispatch must still run: its heartbeat is the only
// liveness signal the fabric has for that agent, so degrading to watch-only is
// correct where refusing to start would trade a silent inbox for a silent agent.
func TestWatchOnlyLoopStillRunsAndHeartbeats(t *testing.T) {
	root := wakeTestRoot(t, AgentConfig{
		ID: "claude-deck", Type: "claude", Command: []string{"claude"},
	})
	sendItem(t, root, "claude-deck", "stranded work")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := RunWakeLoop(ctx, root, "claude-deck", 40*time.Millisecond); err != nil {
		t.Fatalf("watch-only loop returned error: %v", err)
	}
}
