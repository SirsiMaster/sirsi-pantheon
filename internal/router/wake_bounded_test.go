package router

import (
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// The doctor wedge (router item 20260729-132508): `launchctl kickstart -k`
// against a label in `state = spawn scheduled` never returns, so an unbounded
// exec parked `sirsi router doctor --fix` in wait4 for 18 minutes. These tests
// pin the two properties that make that impossible to recur.

func TestRunBoundedKillsAHungChild(t *testing.T) {
	start := time.Now()
	err := runBounded(150*time.Millisecond, "sleep", "30")
	elapsed := time.Since(start)

	if !errors.Is(err, ErrWakeExecTimeout) {
		t.Fatalf("a child that outlives the deadline must report ErrWakeExecTimeout, got %v", err)
	}
	// The point of the fix is that the CALLER gets control back. Allow generous
	// slack for a loaded machine; the failure this guards against is unbounded,
	// so anything near the deadline proves the bound holds.
	if elapsed > 5*time.Second {
		t.Fatalf("runBounded did not return promptly: took %s for a 150ms deadline", elapsed)
	}
}

func TestRunBoundedPassesThroughNormalOutcomes(t *testing.T) {
	if err := runBounded(5*time.Second, "true"); err != nil {
		t.Fatalf("a fast successful command must succeed, got %v", err)
	}
	err := runBounded(5*time.Second, "false")
	if err == nil {
		t.Fatal("a failing command must still report its failure")
	}
	if errors.Is(err, ErrWakeExecTimeout) {
		t.Fatalf("a fast FAILURE must not be misreported as a timeout: %v", err)
	}
}

// The amplifier: waking is per-AGENT, but the pass walks per-ITEM. An agent
// sitting on N open items used to get N adapter invocations in ONE pass — N
// kickstarts against the same label — which is how a single hung launchctl
// became the 18-deep pile. TestWakePassReadyAdapterInvokedOnce does not catch
// this: it sends one item, so it can only ever exercise the across-pass path.
func TestWakePassInvokesOncePerAgentNotPerItem(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("executable: %v", err)
	}
	agent := AgentConfig{ID: "many-items-worker", Type: "qwen", Cwd: t.TempDir(),
		Command: []string{self}, Wake: WakeConfig{Mechanism: WakeCLISpawn}}
	root := wakeTestRoot(t, agent)

	const items = 5
	ids := make([]string, 0, items)
	for i := 0; i < items; i++ {
		ids = append(ids, sendItem(t, root, agent.ID, fmt.Sprintf("item-%d", i)))
	}
	calls, _ := withCountingInvoker(t)

	if _, err := WakePass(root, time.Now().UTC()); err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if n := atomic.LoadInt32(calls); n != 1 {
		t.Fatalf("one agent with %d open items must produce exactly 1 adapter invocation in a pass, got %d", items, n)
	}
	// Deduping the exec must not dedupe the bookkeeping: every item still gets
	// its annotation, or items would silently lose their delivery record.
	for _, id := range ids {
		if got := wakeStatusOf(t, root, id).WakeStatus; got != WakeStatusAttempted {
			t.Fatalf("item %s wake_status = %q, want %q — every item keeps its annotation", id, got, WakeStatusAttempted)
		}
	}
}

// WakeExecTimeout must stay a real ceiling. A zero or absurd value would
// silently restore the unbounded behavior this fix exists to remove.
func TestWakeExecTimeoutIsBounded(t *testing.T) {
	if WakeExecTimeout <= 0 || WakeExecTimeout > time.Minute {
		t.Fatalf("WakeExecTimeout must be a short positive bound, got %s", WakeExecTimeout)
	}
}
