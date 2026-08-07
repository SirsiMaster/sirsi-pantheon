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

	reaped, err := ReapDeadThreads(tmp)
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

// TestReapDeadThreads_ForeignMachineUntouched ensures the reaper never retires a
// thread recorded on a DIFFERENT machine (by stable machine id) — this host
// cannot observe that machine's process table, so its liveness is unknowable.
func TestReapDeadThreads_ForeignMachineUntouched(t *testing.T) {
	tmp := t.TempDir()
	oldMID := getMachineIDFn()
	setMachineIDFn(func() string { return "THIS-MACHINE" })
	defer setMachineIDFn(oldMID)

	// A record stamped with a DIFFERENT machine id is provably foreign. Host is
	// irrelevant to scoping now — only the machine id is.
	thr, _ := RegisterThread(tmp, &Thread{AgentID: "claude-remote", Surface: "claude", PID: 5001, MachineID: "OTHER-MACHINE"})

	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone })
	defer setPIDStateFn(old)

	reaped, err := ReapDeadThreads(tmp)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 0 {
		t.Fatalf("expected 0 reaped for a foreign machine, got %d", len(reaped))
	}
	reg, _ := LoadThreadRegistry(tmp)
	if got := reg.Threads[thr.ThreadID].Status; got != ThreadStatusActive {
		t.Errorf("foreign-machine thread was reaped: status=%q want active", got)
	}
}

// TestReapDeadThreads_StaleHostnameStillReaped is the direct regression for the
// 1d16h stranded inbox: a dead-PID record written under a PRIOR hostname (the
// laptop changed networks: Mac.lan / MacBook-Pro-2.local / Mac.hsd1... are one
// machine) must STILL be reaped. The old host-equality guard skipped it as a
// "foreign host"; machine-id scoping treats an id-less legacy record as local.
func TestReapDeadThreads_StaleHostnameStillReaped(t *testing.T) {
	tmp := t.TempDir()
	oldMID := getMachineIDFn()
	setMachineIDFn(func() string { return "THIS-MACHINE" })
	defer setMachineIDFn(oldMID)

	thr, _ := RegisterThread(tmp, &Thread{AgentID: "claude-pantheon", Surface: "worker", PID: 6001, Host: "MacBook-Pro-2.local"})
	// Force the pre-migration shape: a legacy record with NO machine id, exactly
	// the 355 records the live registry carried.
	reg, _ := LoadThreadRegistry(tmp)
	reg.Threads[thr.ThreadID].MachineID = ""
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveThreadRegistry: %v", err)
	}

	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone })
	defer setPIDStateFn(old)

	reaped, err := ReapDeadThreads(tmp)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	if len(reaped) != 1 {
		t.Fatalf("a dead-PID record under a stale hostname must be reaped, got %d", len(reaped))
	}
	reg, _ = LoadThreadRegistry(tmp)
	if got := reg.Threads[thr.ThreadID].Status; got != ThreadStatusReaped {
		t.Errorf("stale-hostname dead thread: status=%q want reaped", got)
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

	reaped, err := ReapDeadThreads(tmp)
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

func TestRegisterThread_PidRecycleMintsFreshNotReuse(t *testing.T) {
	tmp := t.TempDir()
	// Original session: pid 50000, OS start signature "A".
	first, err := RegisterThread(tmp, &Thread{
		AgentID: "a", Surface: "claude", PID: 50000, StartTime: "A",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The OS later recycles pid 50000 onto a DIFFERENT process (start sig "B").
	// Registering without pinning a thread_id MUST mint a fresh record, not adopt
	// the stale one — the (pid, start_time) reap-key catching PID reuse.
	second, err := RegisterThread(tmp, &Thread{
		AgentID: "a", Surface: "claude", PID: 50000, StartTime: "B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ThreadID == first.ThreadID {
		t.Error("pid recycled onto a new process (different start_time) must NOT reuse the old thread record")
	}

	// Sanity: same pid AND same start sig DOES reuse (the live fast-path).
	again, err := RegisterThread(tmp, &Thread{
		AgentID: "a", Surface: "claude", PID: 50000, StartTime: "B",
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.ThreadID != second.ThreadID {
		t.Error("same (pid, start_time) should reuse the live record, not mint a new one")
	}
}

// TestRegisterThread_ReuseMergesWatches is the regression for the router-fabric bug
// (codex-home item 133134, claude-home confirmed): the (agent_id, pid) reuse path
// updated only LastSeenAt/CurrentItem and silently DROPPED a re-register's new
// --watch values, so an agent could never add (nor narrow) its watch set without
// minting a duplicate thread. Design: REPLACE-when-non-empty (authoritative
// re-declaration; allows narrowing), KEEP-on-empty (a bare heartbeat-style
// re-register must not wipe the live declaration).
func TestRegisterThread_ReuseMergesWatches(t *testing.T) {
	defer probeStubs(t, PIDAlive, "sig-1")()
	tmp := t.TempDir()

	base := func(watches []string) *Thread {
		return &Thread{AgentID: "codex-home", Surface: "codex", PID: 7777, StartTime: "sig-1", Watches: watches}
	}

	first, err := RegisterThread(tmp, base([]string{"codex-home"}))
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	// (1) Re-register the SAME agent+pid+start with MORE watches → same thread, new
	// watches PERSIST (the dropped-values bug).
	second, err := RegisterThread(tmp, base([]string{"codex-home", "claude-home", "claude-finalwishes"}))
	if err != nil {
		t.Fatalf("re-register (add): %v", err)
	}
	if second.ThreadID != first.ThreadID {
		t.Fatalf("reuse must return the SAME thread_id; got %q vs %q", second.ThreadID, first.ThreadID)
	}
	for _, want := range []string{"codex-home", "claude-home", "claude-finalwishes"} {
		if !containsStr(second.Watches, want) {
			t.Errorf("re-register dropped watch %q; got %v", want, second.Watches)
		}
	}

	// (2) Narrowing: re-register with a SMALLER non-empty set → replace, not union.
	third, err := RegisterThread(tmp, base([]string{"codex-home"}))
	if err != nil {
		t.Fatalf("re-register (narrow): %v", err)
	}
	if len(third.Watches) != 1 || third.Watches[0] != "codex-home" {
		t.Errorf("narrowing must REPLACE (not union); got %v", third.Watches)
	}

	// (3) Empty incoming watches (bare heartbeat-style register) must NOT wipe.
	fourth, err := RegisterThread(tmp, base(nil))
	if err != nil {
		t.Fatalf("re-register (empty): %v", err)
	}
	if len(fourth.Watches) != 1 || fourth.Watches[0] != "codex-home" {
		t.Errorf("empty incoming must KEEP existing watches; got %v", fourth.Watches)
	}
}

func containsStr(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// TestReapDeadThreads_SaveFailReturnsNil locks the sirsi-io #18 amendment:
// when SaveThreadRegistry fails after mutations, ReapDeadThreads MUST return
// (nil, err) — not (reaped, err). A caller checking len>0 must not print a
// completion banner for a mutation that was never persisted.
func TestReapDeadThreads_SaveFailReturnsNil(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only dirs")
	}
	tmp := t.TempDir()
	// Register a thread with a dead PID so the reaper has work to do.
	thr, err := RegisterThread(tmp, &Thread{AgentID: "claude-test", Surface: "claude", PID: 9001})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	// Stub PID as gone so the thread is eligible for reaping.
	old := getPIDStateFn()
	setPIDStateFn(func(int) PIDState { return PIDGone })
	defer setPIDStateFn(old)
	_ = thr

	// Make the directory read-only so SaveThreadRegistry fails at CreateTemp.
	if err := os.Chmod(tmp, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(tmp, 0o755) //nolint:errcheck

	reaped, saveErr := ReapDeadThreads(tmp)
	if saveErr == nil {
		t.Fatal("expected save error on read-only dir, got nil")
	}
	if len(reaped) != 0 {
		// The fix: a failed save must return nil so callers don't print a
		// completion banner for an un-persisted mutation.
		t.Errorf("save failure must return nil slice, got %d entries (sirsi-io #18)", len(reaped))
	}
}

// TestReapStrayThreads_SaveFailReturnsNil is the same invariant for ReapStrayThreads
// (sirsi-io #18 amendment — both functions share the same two branches).
func TestReapStrayThreads_SaveFailReturnsNil(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only dirs")
	}
	tmp := t.TempDir()
	// Anchor = alive (PID 9002); stray = dead (PID 9003) so the stray reaper
	// has a non-terminal sibling to sweep. Only the anchor's PID is alive.
	defer perPIDProbe(t, 9002)()
	anchor, err := RegisterThread(tmp, &Thread{AgentID: "claude-test", Surface: "claude", PID: 9002, StartTime: "sig"})
	if err != nil {
		t.Fatalf("register anchor: %v", err)
	}
	// A second record for the same (agent, surface) — stale and dead PID.
	stray := &Thread{
		ThreadID:   "thr-stray-savefail",
		AgentID:    "claude-test",
		Surface:    "claude",
		Status:     ThreadStatusActive,
		PID:        9003,
		MachineID:  anchor.MachineID,
		LastSeenAt: anchor.LastSeenAt.Add(-time.Minute),
		StartedAt:  anchor.StartedAt.Add(-time.Minute),
	}
	reg, _ := LoadThreadRegistry(tmp)
	reg.Threads[stray.ThreadID] = stray
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveThreadRegistry (setup): %v", err)
	}

	// Make the directory read-only so SaveThreadRegistry fails inside Reap.
	if err := os.Chmod(tmp, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(tmp, 0o755) //nolint:errcheck

	retired, saveErr := ReapStrayThreads(tmp)
	if saveErr == nil {
		t.Fatal("expected save error on read-only dir, got nil")
	}
	if len(retired) != 0 {
		t.Errorf("save failure must return nil slice, got %d entries (sirsi-io #18)", len(retired))
	}
}

// TestRegisterThread_AlwaysWatchesSelf locks the A27 contract that a thread always
// watches its OWN inbox, even when the --watch declaration omits self (claude-home
// #76 follow-up: a `--watch other-agent` that drops self would leave the thread
// blind to its own inbox). Holds on both the fresh and reuse paths.
func TestRegisterThread_AlwaysWatchesSelf(t *testing.T) {
	defer probeStubs(t, PIDAlive, "sig-2")()
	tmp := t.TempDir()
	mk := func(watches []string) *Thread {
		return &Thread{AgentID: "claude-pantheon", Surface: "claude", PID: 8888, StartTime: "sig-2", Watches: watches}
	}
	// Fresh register declaring only OTHER agents — self must be added.
	first, err := RegisterThread(tmp, mk([]string{"claude-home", "codex-pantheon"}))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if !containsStr(first.Watches, "claude-pantheon") {
		t.Errorf("fresh register must watch self; got %v", first.Watches)
	}
	// Re-register (reuse path) again omitting self — self must remain.
	second, err := RegisterThread(tmp, mk([]string{"claude-home"}))
	if err != nil {
		t.Fatalf("re-register: %v", err)
	}
	if second.ThreadID != first.ThreadID {
		t.Fatalf("reuse must return same thread_id")
	}
	if !containsStr(second.Watches, "claude-pantheon") {
		t.Errorf("reuse re-register must keep self-watch; got %v", second.Watches)
	}
}

// ── Session-lease tests (mint-churn fix) ─────────────────────────────────
//
// These tests verify the (session_id, surface) reuse path and the
// LeaseSessionTTL expiry path. They are written so that they FAIL against
// the code before this PR:
//
//   BEFORE: RegisterThread keyed only on (agent_id, pid); a pid=0 record
//   skipped the reuse fast-path and minted a fresh thread every call.
//   Running the first test against the old code yields:
//     second.ThreadID != first.ThreadID (FAIL — a fresh record was minted)
//
//   BEFORE: ReapDeadThreads used DefaultThreadStaleAfter (5 min) for all
//   pid<minAgentPID records, including session-keyed ones. Running the second
//   test against the old code yields:
//     session-keyed record reaped after 5 min (FAIL — too short)

// TestRegisterThread_SessionKeyed_Reuse verifies that a second RegisterThread
// call with the same session_id and surface returns the SAME thread record
// (no fresh mint) — the structural fix for ~21 mints/hour on claude-home.
func TestRegisterThread_SessionKeyed_Reuse(t *testing.T) {
	tmp := t.TempDir()
	sessID := "test-session-abc123"

	first, err := RegisterThread(tmp, &Thread{
		AgentID:   "claude-home",
		Surface:   "claude",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatalf("first register: %v", err)
	}

	// Simulate a subsequent hook fire in the same conversation: same session_id,
	// no PID (app-hosted surface has no durable PID). Before this fix, pid=0
	// skips the reuse path and mints a fresh record every call.
	second, err := RegisterThread(tmp, &Thread{
		AgentID:   "claude-home",
		Surface:   "claude",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatalf("second register: %v", err)
	}

	if second.ThreadID != first.ThreadID {
		t.Errorf("session-keyed reuse: got new thread %s, want %s (no fresh mint)", second.ThreadID, first.ThreadID)
	}

	// Verify the registry has exactly one record.
	reg, err := LoadThreadRegistry(tmp)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	active := 0
	for _, thr := range reg.Threads {
		if thr == nil || thr.Status.IsTerminal() {
			continue
		}
		if thr.SessionID == sessID {
			active++
		}
	}
	if active != 1 {
		t.Errorf("expected exactly 1 live session-keyed record, got %d", active)
	}
}

// TestRegisterThread_SessionKeyed_DifferentSessions verifies that two distinct
// session_ids mint two independent records (sessions are not cross-contaminated).
func TestRegisterThread_SessionKeyed_DifferentSessions(t *testing.T) {
	tmp := t.TempDir()

	first, err := RegisterThread(tmp, &Thread{
		AgentID:   "claude-home",
		Surface:   "claude",
		SessionID: "session-A",
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	second, err := RegisterThread(tmp, &Thread{
		AgentID:   "claude-home",
		Surface:   "claude",
		SessionID: "session-B",
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if first.ThreadID == second.ThreadID {
		t.Errorf("different sessions must produce different thread records; both got %s", first.ThreadID)
	}
}

// TestReapDeadThreads_SessionKeyed_UsesLeaseTTL verifies that a session-keyed
// record (SessionID set, PID=0) is NOT reaped before LeaseSessionTTL and IS
// reaped after it. Before this fix, all pid<minAgentPID records used
// DefaultThreadStaleAfter (5 min), so a session-keyed record was reaped after
// 5 min even while the conversation was still live.
func TestReapDeadThreads_SessionKeyed_UsesLeaseTTL(t *testing.T) {
	tmp := t.TempDir()

	reg, _ := LoadThreadRegistry(tmp)
	// Write a session-keyed record whose LastSeenAt is 6 minutes ago
	// (past DefaultThreadStaleAfter but well within LeaseSessionTTL).
	sixMinAgo := time.Now().UTC().Add(-6 * time.Minute)
	reg.Threads["thr-sess-test"] = &Thread{
		ThreadID:   "thr-sess-test",
		AgentID:    "claude-home",
		Surface:    "claude",
		SessionID:  "live-session-xyz",
		Status:     ThreadStatusActive,
		StartedAt:  sixMinAgo,
		LastSeenAt: sixMinAgo,
		MachineID:  MachineID(), // same machine so the reaper checks it
	}
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatalf("save: %v", err)
	}

	reaped, err := ReapDeadThreads(tmp)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	for _, r := range reaped {
		if r.ThreadID == "thr-sess-test" {
			t.Errorf("session-keyed record reaped after 6 min; want it to survive until LeaseSessionTTL (%s)", LeaseSessionTTL)
		}
	}

	// Verify the record is still active.
	reg2, _ := LoadThreadRegistry(tmp)
	thr := reg2.Threads["thr-sess-test"]
	if thr == nil || thr.Status.IsTerminal() {
		t.Errorf("session-keyed record should still be active after 6 min, got %v", thr)
	}
}

// TestReapDeadThreads_SessionKeyed_ExpiresAfterLease verifies that a
// session-keyed record IS reaped once LeaseSessionTTL elapses without renewal.
func TestReapDeadThreads_SessionKeyed_ExpiresAfterLease(t *testing.T) {
	tmp := t.TempDir()

	reg, _ := LoadThreadRegistry(tmp)
	expired := time.Now().UTC().Add(-(LeaseSessionTTL + time.Minute))
	reg.Threads["thr-expired-sess"] = &Thread{
		ThreadID:   "thr-expired-sess",
		AgentID:    "claude-home",
		Surface:    "claude",
		SessionID:  "dead-session-abc",
		Status:     ThreadStatusActive,
		StartedAt:  expired,
		LastSeenAt: expired,
		MachineID:  MachineID(),
	}
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatalf("save: %v", err)
	}

	reaped, err := ReapDeadThreads(tmp)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	found := false
	for _, r := range reaped {
		if r.ThreadID == "thr-expired-sess" {
			found = true
		}
	}
	if !found {
		t.Errorf("session-keyed record not reaped after LeaseSessionTTL (%s) + 1 min", LeaseSessionTTL)
	}
}

// TestRegisterThread_SessionKeyed_RenewsLastSeen verifies that a returning
// hook fire advances LastSeenAt on the reused record (the lease renewal).
func TestRegisterThread_SessionKeyed_RenewsLastSeen(t *testing.T) {
	tmp := t.TempDir()
	sessID := "renew-test-session"

	first, err := RegisterThread(tmp, &Thread{
		AgentID:   "claude-home",
		Surface:   "claude",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatalf("first: %v", err)
	}

	time.Sleep(5 * time.Millisecond)

	second, err := RegisterThread(tmp, &Thread{
		AgentID:   "claude-home",
		Surface:   "claude",
		SessionID: sessID,
	})
	if err != nil {
		t.Fatalf("second: %v", err)
	}

	if !second.LastSeenAt.After(first.LastSeenAt) {
		t.Errorf("lease renewal must advance LastSeenAt; first=%s second=%s", first.LastSeenAt, second.LastSeenAt)
	}
}

// captureSalvageInscriptions swaps the Stele side effect for a recorder and
// restores it on cleanup (Rule A21 accessor pattern — never assign directly).
func captureSalvageInscriptions(t *testing.T) *[]map[string]string {
	t.Helper()
	var got []map[string]string
	old := getInscribeSalvageFn()
	setInscribeSalvageFn(func(_ string, data map[string]string) {
		got = append(got, data)
	})
	t.Cleanup(func() { setInscribeSalvageFn(old) })
	return &got
}

// A stray that is NOT retired must NOT be inscribed as salvaged. The inscription
// used to fire inside the sweep loop, before SaveThreadRegistry decided whether the
// reap happened at all — so a failed save left the Stele claiming a reap that never
// occurred, and a retry of the pass (PR #619) re-inscribed every unpersisted stray.
// codex-home requested proof of this ordering before binding #619.
func TestReapStrayThreads_SaveFailInscribesNothing(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to read-only dirs")
	}
	got := captureSalvageInscriptions(t)
	tmp := t.TempDir()
	defer perPIDProbe(t, 9002)()
	anchor, err := RegisterThread(tmp, &Thread{AgentID: "claude-test", Surface: "claude", PID: 9002, StartTime: "sig"})
	if err != nil {
		t.Fatalf("register anchor: %v", err)
	}
	// The stray carries salvageable state, so it WOULD inscribe if the pass got
	// that far — without this the test would pass vacuously on an empty tombstone.
	stray := &Thread{
		ThreadID:    "thr-stray-noinscribe",
		AgentID:     "claude-test",
		Surface:     "claude",
		Status:      ThreadStatusActive,
		PID:         9003,
		MachineID:   anchor.MachineID,
		CurrentItem: "item-in-flight",
		LastSeenAt:  anchor.LastSeenAt.Add(-time.Minute),
		StartedAt:   anchor.StartedAt.Add(-time.Minute),
	}
	if _, ok := straySalvage(stray, "thr-anchor"); !ok {
		t.Fatal("fixture must be salvageable, else this test proves nothing")
	}
	reg, _ := LoadThreadRegistry(tmp)
	reg.Threads[stray.ThreadID] = stray
	if err := SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveThreadRegistry (setup): %v", err)
	}

	if err := os.Chmod(tmp, 0o555); err != nil { // force the save inside Reap to fail
		t.Fatalf("chmod: %v", err)
	}
	defer os.Chmod(tmp, 0o755) //nolint:errcheck

	if _, err := ReapStrayThreads(tmp); err == nil {
		t.Fatal("expected save error on read-only dir, got nil")
	}
	if len(*got) != 0 {
		t.Errorf("save failed, so nothing was retired — want 0 inscriptions, got %d: %v", len(*got), *got)
	}
}

// The other direction: a stray that IS retired must still be inscribed. Deferring
// the side effect must not quietly drop the "nothing lost" guarantee (owner
// directive 2026-07-22) — a fix that inscribes nothing would pass the test above.
func TestReapStrayThreads_SuccessStillInscribes(t *testing.T) {
	got := captureSalvageInscriptions(t)
	tmp := t.TempDir()
	defer perPIDProbe(t, 9002)()
	anchor, err := RegisterThread(tmp, &Thread{AgentID: "claude-test", Surface: "claude", PID: 9002, StartTime: "sig"})
	if err != nil {
		t.Fatalf("register anchor: %v", err)
	}
	stray := &Thread{
		ThreadID:    "thr-stray-inscribed",
		AgentID:     "claude-test",
		Surface:     "claude",
		Status:      ThreadStatusActive,
		PID:         9003,
		MachineID:   anchor.MachineID,
		CurrentItem: "item-in-flight",
		LastSeenAt:  anchor.LastSeenAt.Add(-time.Minute),
		StartedAt:   anchor.StartedAt.Add(-time.Minute),
	}
	reg, _ := LoadThreadRegistry(tmp)
	reg.Threads[stray.ThreadID] = stray
	if err = SaveThreadRegistry(tmp, reg); err != nil {
		t.Fatalf("SaveThreadRegistry (setup): %v", err)
	}

	retired, err := ReapStrayThreads(tmp)
	if err != nil {
		t.Fatalf("ReapStrayThreads: %v", err)
	}
	if len(retired) != 1 {
		t.Fatalf("want the stray retired, got %d", len(retired))
	}
	if len(*got) != 1 {
		t.Fatalf("a retired salvageable stray must inscribe exactly once, got %d", len(*got))
	}
	if (*got)[0]["thread_id"] != stray.ThreadID {
		t.Errorf("inscribed the wrong thread: %v", (*got)[0])
	}
	// prior_status must be the PRE-mutation status: the payload is computed in the
	// sweep loop, and deferring only the write is what keeps this honest.
	if (*got)[0]["prior_status"] != string(ThreadStatusActive) {
		t.Errorf("prior_status = %q, want %q — payload was computed after mutation",
			(*got)[0]["prior_status"], ThreadStatusActive)
	}
}
