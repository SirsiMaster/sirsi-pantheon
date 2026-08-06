package router

import (
	"os"
	"sync/atomic"
	"testing"
	"time"
)

// Heartbeat freshness is not liveness when the process is PROVABLY gone.
// Observed live 2026-08-03: the claude-pantheon worker died at 21:21Z, 24
// launchd jobs sat down ~15 hours, and CTR reported the lane "heartbeat-fresh"
// the whole time — so its inbox was never woken. A green surface over a dead
// thing, and the reason nobody noticed the fabric was down.

func TestWakePass_DeadPIDIsNotArmed(t *testing.T) {
	agent := AgentConfig{ID: "dead-worker", Type: "gemma",
		Wake: WakeConfig{Mechanism: WakeAPICall, Endpoint: "http://x"}}
	root := wakeTestRoot(t, agent)
	sendItem(t, root, agent.ID, "work waiting on a dead worker")
	armAgent(t, root, agent.ID) // fresh heartbeat, real pid

	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone }) // the process died
	defer setPIDStateFn(old)

	calls, _ := withCountingInvoker(t)
	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if len(rep.Armed) != 0 {
		t.Fatalf("a provably-dead process must not count as armed, got %d armed outcome(s)", len(rep.Armed))
	}
	if atomic.LoadInt32(calls) == 0 {
		t.Fatal("a dead worker's inbox must be WOKEN — heartbeat freshness kept it silently stranded for 15 hours")
	}
}

// Only PROVABLY gone demotes. An unreadable process table (EPERM → PIDUnknown)
// must not strand a live lane — the same asymmetry ReapDeadThreads uses.
func TestWakePass_UnknownPIDStaysArmed(t *testing.T) {
	agent := AgentConfig{ID: "unknown-worker", Type: "gemma",
		Wake: WakeConfig{Mechanism: WakeAPICall, Endpoint: "http://x"}}
	root := wakeTestRoot(t, agent)
	sendItem(t, root, agent.ID, "item")
	armAgent(t, root, agent.ID)

	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDUnknown })
	defer setPIDStateFn(old)

	calls, _ := withCountingInvoker(t)
	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if len(rep.Armed) != 1 {
		t.Fatalf("PIDUnknown must keep the old heartbeat behavior, got %d armed", len(rep.Armed))
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("an armed agent must not be woken, invoker called %d times", n)
	}
}

// A live process stays armed — the fix must not re-wake a working lane.
func TestWakePass_LivePIDStaysArmed(t *testing.T) {
	agent := AgentConfig{ID: "live-worker", Type: "gemma",
		Wake: WakeConfig{Mechanism: WakeAPICall, Endpoint: "http://x"}}
	root := wakeTestRoot(t, agent)
	sendItem(t, root, agent.ID, "item")
	armAgent(t, root, agent.ID) // registers with os.Getpid(), genuinely alive

	calls, _ := withCountingInvoker(t)
	rep, err := WakePass(root, time.Now().UTC())
	if err != nil {
		t.Fatalf("WakePass: %v", err)
	}
	if len(rep.Armed) != 1 {
		t.Fatalf("a live pid (%d) must stay armed, got %d", os.Getpid(), len(rep.Armed))
	}
	if n := atomic.LoadInt32(calls); n != 0 {
		t.Fatalf("a live armed agent must not be woken, invoker called %d times", n)
	}
}
