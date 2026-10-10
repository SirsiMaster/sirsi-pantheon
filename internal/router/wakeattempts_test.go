package router

import (
	"errors"
	"testing"
	"time"
)

// Round trip of the local tracker itself, independent of a wake pass.
func TestWakeAttemptsRoundTrip(t *testing.T) {
	root := t.TempDir()
	if got := TerminalWakeFailures(root, "ghost"); got != 0 {
		t.Fatalf("unwritten tracker: got %d, want 0", got)
	}
	recordWakeMiss(root, "flaky")
	recordWakeMiss(root, "flaky")
	if got := TerminalWakeFailures(root, "flaky"); got != 2 {
		t.Fatalf("after 2 misses: got %d, want 2", got)
	}
	recordWakeSuccess(root, "flaky")
	if got := TerminalWakeFailures(root, "flaky"); got != 0 {
		t.Fatalf("after success: got %d, want 0 (streak reset)", got)
	}
}

// A real WakePass whose invoker always fails must increment the per-agent
// miss count every pass (one invoke per agent per pass, per the existing
// dedup), and a later pass whose invoker succeeds must reset it to zero —
// the exact option (b) slice from lane-ping-and-incoming-notice remaining (4).
func TestWakePassTracksConsecutiveMisses(t *testing.T) {
	agent := AgentConfig{ID: "miss-agent", Type: "qwen", Wake: WakeConfig{Mechanism: WakeAPICall, Endpoint: "http://x"}}
	root := wakeTestRoot(t, agent)
	sendItem(t, root, agent.ID, "miss-1")

	old := getWakeInvoke()
	t.Cleanup(func() { setWakeInvoke(old) })
	failing := errors.New("adapter unreachable")
	setWakeInvoke(func(cfg AgentConfig, adapter string) error { return failing })

	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		// Force re-invocation each pass: idempotent skip only applies to a
		// successful wake_status:wake-attempted, which a failing invoker never
		// writes (it writes wake-unavailable instead), so no retry-after jump
		// is needed here.
		if _, err := WakePass(root, now.Add(time.Duration(i)*time.Hour)); err != nil {
			t.Fatalf("WakePass pass %d: %v", i, err)
		}
	}
	if got := TerminalWakeFailures(root, agent.ID); got != 3 {
		t.Fatalf("after 3 failing passes: got %d, want 3", got)
	}

	setWakeInvoke(func(cfg AgentConfig, adapter string) error { return nil })
	if _, err := WakePass(root, now.Add(4*time.Hour)); err != nil {
		t.Fatalf("WakePass success pass: %v", err)
	}
	if got := TerminalWakeFailures(root, agent.ID); got != 0 {
		t.Fatalf("after a successful pass: got %d, want 0 (streak reset)", got)
	}
}
