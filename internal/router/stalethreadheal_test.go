package router

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newTestRouter creates a minimal router directory for test use.
func newTestRouterRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	routerRoot := filepath.Join(dir, ".agents", "idea-router")
	if err := os.MkdirAll(routerRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	return routerRoot
}

func TestRunStaleThreadReconcileDuty_ReapsPhantomPID(t *testing.T) {
	// A thread with PID=0 and a stale heartbeat is a phantom: ReapDeadThreads
	// retires it to `reaped` so it no longer appears in `sirsi thread list`
	// (reaped records are terminal and excluded by default). This clears the
	// EffectiveStale / registry-police alarm.
	routerRoot := newTestRouterRoot(t)
	now := time.Now().UTC()

	thr := &Thread{
		ThreadID:   "thr-phantom-001",
		AgentID:    "claude-test",
		Surface:    "claude",
		PID:        0, // phantom: no live OS process
		Status:     ThreadStatusActive,
		StartedAt:  now.Add(-2 * time.Hour),
		LastSeenAt: now.Add(-30 * time.Minute), // well past DefaultThreadStaleAfter
	}
	reg := &ThreadRegistry{Threads: map[string]*Thread{thr.ThreadID: thr}}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}

	if err := RunStaleThreadReconcileDuty(routerRoot, t.TempDir()); err != nil {
		t.Fatalf("duty returned error: %v", err)
	}

	reg2, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	got := reg2.Threads[thr.ThreadID]
	if got == nil {
		t.Fatal("thread was removed (expected reaped, not deleted)")
	}
	// ReapDeadThreads retires phantom (PID=0) stale threads to reaped.
	if got.Status != ThreadStatusReaped {
		t.Errorf("expected status=reaped for phantom stale thread, got %s", got.Status)
	}
	// No successor should be minted for a phantom reap in supervisor context.
	for id, other := range reg2.Threads {
		if id == thr.ThreadID {
			continue
		}
		if other.SuspendPayload != nil && other.SuspendPayload.ReapedFrom == thr.ThreadID {
			t.Errorf("unexpected successor %s minted for phantom reap — supervisor duty must not mint successors", id)
		}
	}
}

func TestRunStaleThreadReconcileDuty_SkipsAliveSession(t *testing.T) {
	// A thread whose PID is still alive (harness-gated heartbeat) must NOT be
	// suspended even if its heartbeat timestamp has aged out. ReconcileExits has
	// an explicit guard for this: leave it active, just quiet.
	routerRoot := newTestRouterRoot(t)
	now := time.Now().UTC()

	// Use an out-of-range PID (1) to avoid real OS probing, then mock alive.
	// (minAgentPID=2, so PID=1 would be handled as phantom; use a valid PID instead
	// and stub the PID state fn so the test is deterministic.)
	const alivePID = 55555
	old := getPIDStateFn()
	setPIDStateFn(func(pid int) PIDState {
		if pid == alivePID {
			return PIDAlive
		}
		return old(pid)
	})
	defer setPIDStateFn(nil)

	thr := &Thread{
		ThreadID:   "thr-alive-001",
		AgentID:    "claude-home",
		Surface:    "claude",
		PID:        alivePID,
		Status:     ThreadStatusActive,
		StartedAt:  now.Add(-1 * time.Hour),
		LastSeenAt: now.Add(-30 * time.Minute), // stale heartbeat but PID alive
	}
	reg := &ThreadRegistry{Threads: map[string]*Thread{thr.ThreadID: thr}}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}

	if err := RunStaleThreadReconcileDuty(routerRoot, t.TempDir()); err != nil {
		t.Fatalf("duty returned error: %v", err)
	}

	reg2, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	got := reg2.Threads[thr.ThreadID]
	if got == nil {
		t.Fatal("thread was removed, expected active")
	}
	if got.Status != ThreadStatusActive {
		t.Errorf("alive-PID stale thread status changed to %s �� must stay active (harness-gated heartbeat)", got.Status)
	}
}

func TestRunStaleThreadReconcileDuty_SkipsReaped(t *testing.T) {
	// An already-reaped record must stay reaped. The duty must not mint a
	// successor (no transcript access in supervisor context).
	routerRoot := newTestRouterRoot(t)
	now := time.Now().UTC()

	reaped := &Thread{
		ThreadID:   "thr-reaped-001",
		AgentID:    "codex-test",
		Surface:    "codex",
		PID:        0,
		Status:     ThreadStatusReaped,
		StartedAt:  now.Add(-3 * time.Hour),
		LastSeenAt: now.Add(-2 * time.Hour),
		LastError:  "reaped: test sentinel",
	}
	reg := &ThreadRegistry{Threads: map[string]*Thread{reaped.ThreadID: reaped}}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}

	if err := RunStaleThreadReconcileDuty(routerRoot, t.TempDir()); err != nil {
		t.Fatalf("duty returned error: %v", err)
	}

	reg2, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	got := reg2.Threads[reaped.ThreadID]
	if got == nil {
		t.Fatal("reaped thread was removed")
	}
	if got.Status != ThreadStatusReaped {
		t.Errorf("reaped thread status changed to %s — must stay reaped", got.Status)
	}
	// No successor minted — supervisor context has no transcript.
	for id, other := range reg2.Threads {
		if id == reaped.ThreadID {
			continue
		}
		if other.SuspendPayload != nil && other.SuspendPayload.ReapedFrom == reaped.ThreadID {
			t.Errorf("successor %s was minted for reaped thread — supervisor context must not mint successors (no transcript)", id)
		}
	}
}

func TestRunStaleThreadReconcileDuty_SkipsFreshThread(t *testing.T) {
	// A recently-heartbeating thread must not be touched.
	routerRoot := newTestRouterRoot(t)
	now := time.Now().UTC()

	fresh := &Thread{
		ThreadID:   "thr-fresh-001",
		AgentID:    "gemma-test",
		Surface:    "gemma",
		PID:        0,
		Status:     ThreadStatusActive,
		StartedAt:  now.Add(-1 * time.Minute),
		LastSeenAt: now.Add(-30 * time.Second), // well within stale window
	}
	reg := &ThreadRegistry{Threads: map[string]*Thread{fresh.ThreadID: fresh}}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}

	if err := RunStaleThreadReconcileDuty(routerRoot, t.TempDir()); err != nil {
		t.Fatalf("duty returned error: %v", err)
	}

	reg2, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	got := reg2.Threads[fresh.ThreadID]
	if got == nil {
		t.Fatal("fresh thread was removed")
	}
	if got.Status != ThreadStatusActive {
		t.Errorf("fresh thread status changed to %s — must stay active", got.Status)
	}
}

func TestRunStaleThreadReconcileDuty_SkipsSuspended(t *testing.T) {
	// A suspended thread must not be touched — its resume payload is precious.
	routerRoot := newTestRouterRoot(t)
	now := time.Now().UTC()

	suspended := &Thread{
		ThreadID:   "thr-susp-001",
		AgentID:    "claude-home",
		Surface:    "claude",
		PID:        0,
		Status:     ThreadStatusSuspended,
		StartedAt:  now.Add(-4 * time.Hour),
		LastSeenAt: now.Add(-3 * time.Hour),
		SuspendPayload: &SuspendPayload{
			ResumePrompt: "continue pantheon test",
			SuspendedAt:  now.Add(-3 * time.Hour),
		},
	}
	reg := &ThreadRegistry{Threads: map[string]*Thread{suspended.ThreadID: suspended}}
	if err := SaveThreadRegistry(routerRoot, reg); err != nil {
		t.Fatal(err)
	}

	if err := RunStaleThreadReconcileDuty(routerRoot, t.TempDir()); err != nil {
		t.Fatalf("duty returned error: %v", err)
	}

	reg2, err := LoadThreadRegistry(routerRoot)
	if err != nil {
		t.Fatal(err)
	}
	got := reg2.Threads[suspended.ThreadID]
	if got == nil {
		t.Fatal("suspended thread was removed")
	}
	if got.Status != ThreadStatusSuspended {
		t.Errorf("suspended thread status changed to %s — must stay suspended", got.Status)
	}
	if got.SuspendPayload == nil || got.SuspendPayload.ResumePrompt != "continue pantheon test" {
		t.Error("suspended thread payload was mutated")
	}
}

func TestRunStaleThreadReconcileDuty_DutyRegistered(t *testing.T) {
	// Verify the "stale-thread-reconcile" duty exists in supervisorDuties().
	found := false
	for _, d := range supervisorDuties() {
		if d.Name == "stale-thread-reconcile" {
			found = true
			if d.GoRun == nil {
				t.Error("stale-thread-reconcile duty must use GoRun, not a shell script")
			}
			if d.Cadence <= 0 {
				t.Error("stale-thread-reconcile duty must have a positive cadence")
			}
		}
	}
	if !found {
		t.Error("stale-thread-reconcile duty not registered in supervisorDuties()")
	}
}
