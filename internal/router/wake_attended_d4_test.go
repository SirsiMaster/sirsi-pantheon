package router

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ADR-079 D4 close-session negative fixture. A declared-attended lane with a
// real command consumer: its wake-loop holds dispatch, so it must not publish
// ConsumerCapable. With no live attended session, WakePass must report the lane
// needs-owner (Unavailable), never armed.
func TestAttendedLaneWithClosedSessionIsNeedsOwnerNotArmed(t *testing.T) {
	log := filepath.Join(t.TempDir(), "fired.txt")
	script := recordingConsumer(t, log, 0)
	root := wakeTestRoot(t, AgentConfig{
		ID: "ra-like", Type: "claude", Attended: true,
		Wake:     WakeConfig{Mechanism: WakeSessionMessage},
		Consumer: ConsumerConfig{Command: []string{script, consumerAgentPlaceholder}},
	})
	id := sendItem(t, root, "ra-like", "work for the attended session")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	if err := RunWakeLoop(ctx, root, "ra-like", 40*time.Millisecond); err != nil {
		t.Fatalf("RunWakeLoop: %v", err)
	}
	if lines := consumerLines(t, log); len(lines) != 0 {
		t.Fatalf("attended lane dispatched a headless consumer: %v", lines)
	}
	reg, err := LoadThreadRegistry(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, tr := range reg.Threads {
		if tr != nil && tr.AgentID == "ra-like" && tr.Surface == surfaceWorker && tr.ConsumerCapable {
			t.Fatal("attended lane's wake-loop published ConsumerCapable; it holds dispatch, so it would arm the lane with no session")
		}
	}

	// No attended session is registered: the session is closed.
	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	for _, o := range rep.Armed {
		if o.ItemID == id {
			t.Fatal("lane reads armed with its attended session closed; D4 requires needs-owner")
		}
	}
	var found bool
	for _, o := range rep.Unavailable {
		if o.ItemID == id && strings.Contains(o.Detail, "needs-owner") {
			found = true
		}
	}
	if !found {
		t.Fatalf("want the item wake-unavailable with a needs-owner detail; unavailable=%+v", rep.Unavailable)
	}
}

func TestWakeLoopConsumerCapable(t *testing.T) {
	cmd := &ResolvedConsumer{Argv: []string{"x"}}
	for _, c := range []struct {
		name     string
		consumer *ResolvedConsumer
		attended bool
		want     bool
	}{
		{"command consumer", cmd, false, true},
		{"attended lane holds dispatch", cmd, true, false},
		{"resident", &ResolvedConsumer{Resident: true}, false, false},
		{"no consumer", nil, false, false},
	} {
		if got := wakeLoopConsumerCapable(c.consumer, c.attended); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
