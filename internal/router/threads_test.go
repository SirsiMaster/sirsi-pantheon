package router

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadThreadRegistry_Empty(t *testing.T) {
	tmp := t.TempDir()
	reg, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadThreadRegistry: %v", err)
	}
	if len(reg.Threads) != 0 {
		t.Errorf("expected empty registry, got %d", len(reg.Threads))
	}
}

func TestRegisterAndHeartbeatThread(t *testing.T) {
	tmp := t.TempDir()
	thr, err := RegisterThread(tmp, &Thread{
		AgentID: "claude-pantheon",
		Surface: "claude",
		Repo:    "/repo",
		Watches: []string{"claude-pantheon"},
	})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	if thr.ThreadID == "" {
		t.Fatal("expected generated thread_id")
	}
	if thr.Status != ThreadStatusActive {
		t.Errorf("expected active status, got %q", thr.Status)
	}
	if thr.StartedAt.IsZero() || thr.LastSeenAt.IsZero() {
		t.Errorf("timestamps not set")
	}

	// Heartbeat should advance last_seen_at
	time.Sleep(10 * time.Millisecond)
	item := "20260520-test"
	updated, err := Heartbeat(tmp, thr.ThreadID, HeartbeatUpdate{
		Status:      ThreadStatusIdle,
		CurrentItem: &item,
	})
	if err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if !updated.LastSeenAt.After(thr.LastSeenAt) {
		t.Errorf("last_seen_at did not advance")
	}
	if updated.Status != ThreadStatusIdle {
		t.Errorf("status not updated")
	}
	if updated.CurrentItem != "20260520-test" {
		t.Errorf("current_item not updated")
	}

	// Persisted on disk
	if _, err := os.Stat(filepath.Join(tmp, "threads.json")); err != nil {
		t.Errorf("threads.json missing: %v", err)
	}
}

func TestRegisterThread_RequiresAgentAndSurface(t *testing.T) {
	tmp := t.TempDir()
	if _, err := RegisterThread(tmp, &Thread{Surface: "claude"}); err == nil {
		t.Error("expected error for missing agent_id")
	}
	if _, err := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon"}); err == nil {
		t.Error("expected error for missing surface")
	}
}

func TestHeartbeat_UnknownThread(t *testing.T) {
	tmp := t.TempDir()
	if _, err := Heartbeat(tmp, "thr-missing", HeartbeatUpdate{}); err == nil {
		t.Error("expected error for unknown thread")
	}
}

func TestCloseThread(t *testing.T) {
	tmp := t.TempDir()
	thr, _ := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon", Surface: "claude"})
	closed, err := CloseThread(tmp, thr.ThreadID)
	if err != nil {
		t.Fatalf("CloseThread: %v", err)
	}
	if closed.Status != ThreadStatusClosed {
		t.Errorf("expected closed status, got %q", closed.Status)
	}
	// Closed threads are not stale.
	if closed.IsStale(time.Now().Add(24*time.Hour), time.Minute) {
		t.Errorf("closed thread should not be considered stale")
	}
}

// TestHeartbeat_ClosedIsTerminal locks in the reaped-is-terminal guard: a
// closed/reaped thread must not be revived to active by a late heartbeat.
// Regression for CTR false-active resurrection (router item
// 20260601-024355-codex-pantheon-claude-home-execute-fix-ctr-false-active-...).
func TestHeartbeat_ClosedIsTerminal(t *testing.T) {
	tmp := t.TempDir()
	thr, _ := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon", Surface: "claude"})
	if _, err := CloseThread(tmp, thr.ThreadID); err != nil {
		t.Fatalf("CloseThread: %v", err)
	}
	// A late heartbeat against the closed record must be rejected.
	if _, err := Heartbeat(tmp, thr.ThreadID, HeartbeatUpdate{Status: ThreadStatusActive}); err == nil {
		t.Fatalf("expected heartbeat on closed thread to be rejected, got nil error")
	}
	// And the record must remain closed — not silently revived to active.
	reg, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadThreadRegistry: %v", err)
	}
	if got := reg.Threads[thr.ThreadID].Status; got != ThreadStatusClosed {
		t.Errorf("closed thread was revived: status=%q, want %q", got, ThreadStatusClosed)
	}
}

// TestReapDeadThreads_DefunctAndGone locks in Bug B2: a thread whose PID is
// defunct (zombie Z) OR gone must be reaped to the terminal `reaped` status —
// not left `active`. A naive kill -0 check answers "alive" for a zombie, which
// is the exact false-active the registry suffered. Uses an injected prober so
// no real zombie is needed.
func TestReapDeadThreads_DefunctAndGone(t *testing.T) {
	tmp := t.TempDir()
	host, _ := os.Hostname()

	mk := func(agent string, pid int) *Thread {
		thr, err := RegisterThread(tmp, &Thread{AgentID: agent, Surface: "claude", PID: pid, Host: host})
		if err != nil {
			t.Fatalf("RegisterThread(%s): %v", agent, err)
		}
		return thr
	}
	gone := mk("claude-gone", 4001)
	defunct := mk("claude-defunct", 4002)
	alive := mk("claude-alive", 4003)
	noPID := mk("claude-nopid", 0)
	pidOne := mk("claude-pid-one", 1)

	old := getPIDStateFn()
	setPIDStateFn(func(pid int) PIDState {
		switch pid {
		case 4001:
			return PIDGone
		case 4002:
			return PIDDefunct
		case 4003:
			return PIDAlive
		default:
			return PIDUnknown
		}
	})
	defer setPIDStateFn(old)

	reaped, err := ReapDeadThreads(tmp, host)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 2 {
		t.Fatalf("expected 2 reaped (gone+defunct), got %d: %+v", len(reaped), reaped)
	}

	reg, _ := LoadThreadRegistry(tmp)
	if got := reg.Threads[gone.ThreadID].Status; got != ThreadStatusReaped {
		t.Errorf("gone thread: status=%q want reaped", got)
	}
	if got := reg.Threads[defunct.ThreadID].Status; got != ThreadStatusReaped {
		t.Errorf("defunct (zombie) thread: status=%q want reaped — Bug B2 regression", got)
	}
	if got := reg.Threads[alive.ThreadID].Status; got != ThreadStatusActive {
		t.Errorf("alive thread: status=%q want active (must not be reaped)", got)
	}
	if got := reg.Threads[noPID.ThreadID].Status; got != ThreadStatusActive {
		t.Errorf("no-PID thread: status=%q want active (unverifiable, never reaped)", got)
	}
	if got := reg.Threads[pidOne.ThreadID].Status; got != ThreadStatusActive {
		t.Errorf("PID 1 thread: status=%q want active (below PID floor, never reaped)", got)
	}

	// A late heartbeat against a reaped thread must be refused (no revival).
	if _, err := Heartbeat(tmp, defunct.ThreadID, HeartbeatUpdate{Status: ThreadStatusActive}); err == nil {
		t.Error("expected heartbeat on reaped thread to be rejected")
	}
}

// TestReapDeadThreads_RemoteHostUntouched ensures the reaper never retires a
// thread recorded on a different host — we cannot observe a remote process
// table, so its liveness is unknowable here.
func TestReapDeadThreads_RemoteHostUntouched(t *testing.T) {
	tmp := t.TempDir()
	thr, _ := RegisterThread(tmp, &Thread{AgentID: "claude-remote", Surface: "claude", PID: 5001, Host: "some-other-host"})

	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone })
	defer setPIDStateFn(old)

	host, _ := os.Hostname()
	reaped, err := ReapDeadThreads(tmp, host)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 0 {
		t.Fatalf("expected 0 reaped for remote host, got %d", len(reaped))
	}
	reg, _ := LoadThreadRegistry(tmp)
	if got := reg.Threads[thr.ThreadID].Status; got != ThreadStatusActive {
		t.Errorf("remote-host thread was reaped: status=%q want active", got)
	}
}

func TestIsStale(t *testing.T) {
	thr := &Thread{
		Status:     ThreadStatusActive,
		LastSeenAt: time.Now().Add(-10 * time.Minute),
	}
	if !thr.IsStale(time.Now(), 5*time.Minute) {
		t.Error("expected stale")
	}
	thr.LastSeenAt = time.Now()
	if thr.IsStale(time.Now(), 5*time.Minute) {
		t.Error("expected fresh")
	}
}

func TestSortedThreads_NewestFirst(t *testing.T) {
	tmp := t.TempDir()
	a, _ := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon", Surface: "claude"})
	time.Sleep(10 * time.Millisecond)
	b, _ := RegisterThread(tmp, &Thread{AgentID: "codex-pantheon", Surface: "codex"})

	reg, _ := LoadThreadRegistry(tmp)
	sorted := reg.SortedThreads()
	if len(sorted) != 2 {
		t.Fatalf("expected 2 threads, got %d", len(sorted))
	}
	if sorted[0].ThreadID != b.ThreadID {
		t.Errorf("expected newest first, got %s before %s", sorted[0].ThreadID, b.ThreadID)
	}
	_ = a
}

func TestPruneClosed(t *testing.T) {
	tmp := t.TempDir()
	thr, _ := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon", Surface: "claude"})
	_, _ = CloseThread(tmp, thr.ThreadID)
	reg, _ := LoadThreadRegistry(tmp)
	// Set last_seen far in the past so it's prunable.
	reg.Threads[thr.ThreadID].LastSeenAt = time.Now().Add(-48 * time.Hour)
	removed := reg.PruneClosed(time.Now(), 24*time.Hour)
	if removed != 1 {
		t.Errorf("expected 1 pruned, got %d", removed)
	}
	if len(reg.Threads) != 0 {
		t.Errorf("expected empty after prune")
	}
}

func TestNewThreadID_Unique(t *testing.T) {
	a := NewThreadID()
	b := NewThreadID()
	if a == b {
		t.Errorf("expected unique IDs, got %s == %s", a, b)
	}
}

// TestRegisterThread_IdempotentOnAgentPID locks in the fix for the heartbeat-loop
// explosion: re-registering the same live (agent_id, pid) must reuse the existing
// thread instead of minting a new one (and a new caffeinate loop). Regression guard
// for the 160-loops-for-10-PIDs defect.
func TestRegisterThread_IdempotentOnAgentPID(t *testing.T) {
	tmp := t.TempDir()
	first, err := RegisterThread(tmp, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", Repo: "/repo", PID: 4242,
	})
	if err != nil {
		t.Fatalf("first RegisterThread: %v", err)
	}

	// Same session re-registers (discover/police churn). No pinned ThreadID.
	second, err := RegisterThread(tmp, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", Repo: "/repo", PID: 4242,
		CurrentItem: "20260601-resume",
	})
	if err != nil {
		t.Fatalf("second RegisterThread: %v", err)
	}
	if second.ThreadID != first.ThreadID {
		t.Errorf("expected reuse of %s, got new thread %s", first.ThreadID, second.ThreadID)
	}
	if second.CurrentItem != "20260601-resume" {
		t.Errorf("current_item not carried onto reused thread: %q", second.CurrentItem)
	}

	reg, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatalf("LoadThreadRegistry: %v", err)
	}
	if len(reg.Threads) != 1 {
		t.Errorf("expected 1 thread after re-register, got %d", len(reg.Threads))
	}

	// A different PID is a genuinely different session — must NOT collapse.
	other, err := RegisterThread(tmp, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", Repo: "/repo", PID: 9999,
	})
	if err != nil {
		t.Fatalf("other RegisterThread: %v", err)
	}
	if other.ThreadID == first.ThreadID {
		t.Errorf("distinct PID collapsed into existing thread")
	}

	// A terminal record must not be reused — closing then re-registering starts fresh.
	if _, hbErr := Heartbeat(tmp, first.ThreadID, HeartbeatUpdate{Status: ThreadStatusClosed}); hbErr != nil {
		t.Fatalf("Heartbeat close: %v", hbErr)
	}
	revived, err := RegisterThread(tmp, &Thread{
		AgentID: "claude-pantheon", Surface: "claude", Repo: "/repo", PID: 4242,
	})
	if err != nil {
		t.Fatalf("revive RegisterThread: %v", err)
	}
	if revived.ThreadID == first.ThreadID {
		t.Errorf("reused a closed (terminal) thread; expected fresh thread_id")
	}
}

func TestReapDeadThreads_PidSanityFloor(t *testing.T) {
	tmp := t.TempDir()
	now := time.Now().UTC()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		// pid=0 phantom, stale → reaped.
		"thr-phantom0": {
			ThreadID: "thr-phantom0", AgentID: "a", Surface: "claude",
			Status: ThreadStatusActive, PID: 0, LastSeenAt: now.Add(-2 * time.Hour),
		},
		// pid=1 (launchd), stale → reaped.
		"thr-launchd1": {
			ThreadID: "thr-launchd1", AgentID: "a", Surface: "claude",
			Status: ThreadStatusActive, PID: 1, LastSeenAt: now.Add(-2 * time.Hour),
		},
		// pid-less surface that is FRESHLY heartbeating → must NOT be reaped.
		"thr-pidless-fresh": {
			ThreadID: "thr-pidless-fresh", AgentID: "b", Surface: "mcp",
			Status: ThreadStatusActive, PID: 0, LastSeenAt: now.Add(-10 * time.Second),
		},
	}}
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatal(err)
	}

	reaped, err := ReapDeadThreads(tmp, "")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range reaped {
		got[r.ThreadID] = true
	}
	if !got["thr-phantom0"] || !got["thr-launchd1"] {
		t.Errorf("stale phantom pid<=1 records should be reaped, got %+v", reaped)
	}
	if got["thr-pidless-fresh"] {
		t.Error("a freshly-heartbeating pid-less surface must NOT be reaped")
	}

	out, _ := LoadThreadRegistry(tmp)
	if out.Threads["thr-phantom0"].Status != ThreadStatusReaped {
		t.Error("phantom0 should be persisted as reaped")
	}
	if out.Threads["thr-pidless-fresh"].Status != ThreadStatusActive {
		t.Error("fresh pid-less surface should stay active")
	}
}

func TestRegisterThread_CompactsOldTerminalRecords(t *testing.T) {
	tmp := t.TempDir()
	now := time.Now().UTC()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		// Old terminal (reaped) — past TerminalRetention → compacted.
		"thr-old-reaped": {
			ThreadID: "thr-old-reaped", AgentID: "a", Surface: "claude",
			Status: ThreadStatusReaped, LastSeenAt: now.Add(-5 * 24 * time.Hour),
		},
		// Recent terminal (closed) — within retention → kept.
		"thr-recent-closed": {
			ThreadID: "thr-recent-closed", AgentID: "a", Surface: "claude",
			Status: ThreadStatusClosed, LastSeenAt: now.Add(-1 * time.Hour),
		},
		// Ancient but ACTIVE — must NEVER be compacted (only terminal records are).
		"thr-active": {
			ThreadID: "thr-active", AgentID: "a", Surface: "claude",
			Status: ThreadStatusActive, PID: 999999, LastSeenAt: now.Add(-100 * 24 * time.Hour),
		},
	}}
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatal(err)
	}

	// A fresh register triggers the opportunistic compaction.
	if _, err := RegisterThread(tmp, &Thread{AgentID: "b", Surface: "claude"}); err != nil {
		t.Fatal(err)
	}

	out, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out.Threads["thr-old-reaped"]; ok {
		t.Error("old reaped record should be compacted on register")
	}
	if _, ok := out.Threads["thr-recent-closed"]; !ok {
		t.Error("recent closed record (within retention) should be kept")
	}
	if _, ok := out.Threads["thr-active"]; !ok {
		t.Error("active record must NEVER be compacted, even if ancient")
	}
}
