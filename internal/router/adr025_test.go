package router

import (
	"testing"
	"time"
)

func mustRegister(t *testing.T, root, agent string, pid int) *Thread {
	t.Helper()
	thr, err := RegisterThread(root, &Thread{AgentID: agent, Surface: "claude", PID: pid})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	return thr
}

// ADR-025 test 1: suspend is idempotent and snapshots the payload.
func TestSuspendThread_IdempotentWithPayload(t *testing.T) {
	root := t.TempDir()
	thr := mustRegister(t, root, "claude-pantheon", 4242)

	payload := &SuspendPayload{ThothRef: "stele-abc", OwnedOpenItems: []string{"item-1"}, ResumePrompt: "Pantheon Native App"}
	s1, err := SuspendThread(root, thr.ThreadID, payload)
	if err != nil {
		t.Fatalf("SuspendThread: %v", err)
	}
	if s1.Status != ThreadStatusSuspended {
		t.Errorf("status = %q, want suspended", s1.Status)
	}
	if s1.SuspendPayload == nil || s1.SuspendPayload.ThothRef != "stele-abc" || s1.SuspendPayload.SuspendedAt.IsZero() {
		t.Errorf("payload not snapshotted: %+v", s1.SuspendPayload)
	}

	// Idempotent: suspending again returns it unchanged, no error.
	s2, err := SuspendThread(root, thr.ThreadID, nil)
	if err != nil {
		t.Fatalf("second SuspendThread: %v", err)
	}
	if s2.Status != ThreadStatusSuspended || s2.SuspendPayload == nil || s2.SuspendPayload.ThothRef != "stele-abc" {
		t.Error("second suspend must be a no-op preserving the original payload")
	}
}

// ADR-025 test 2: resume restores payload to the caller and flips to active.
func TestResumeThread_RestoresPayloadAndActivates(t *testing.T) {
	root := t.TempDir()
	thr := mustRegister(t, root, "claude-pantheon", 4243)
	_, _ = SuspendThread(root, thr.ThreadID, &SuspendPayload{OwnedOpenItems: []string{"i1", "i2"}, ResumePrompt: "resume me"})

	resumed, err := ResumeThread(root, thr.ThreadID)
	if err != nil {
		t.Fatalf("ResumeThread: %v", err)
	}
	if resumed.Status != ThreadStatusActive {
		t.Errorf("status = %q, want active", resumed.Status)
	}
	if resumed.SuspendPayload == nil || len(resumed.SuspendPayload.OwnedOpenItems) != 2 {
		t.Error("resume must return the payload so the caller can re-surface owned items")
	}
	// Persisted record has the payload cleared.
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].SuspendPayload != nil {
		t.Error("persisted suspend_payload must be cleared after resume")
	}
	// Resuming a non-suspended thread errors.
	if _, err := ResumeThread(root, thr.ThreadID); err == nil {
		t.Error("resuming an active thread must error")
	}
}

// ADR-025 test 3a: heartbeat rejects suspended (no revive, no last_seen refresh).
func TestHeartbeat_RejectsSuspended(t *testing.T) {
	root := t.TempDir()
	thr := mustRegister(t, root, "claude-pantheon", 4244)
	suspended, _ := SuspendThread(root, thr.ThreadID, nil)
	before := suspended.LastSeenAt

	if _, err := Heartbeat(root, thr.ThreadID, HeartbeatUpdate{}); err == nil {
		t.Fatal("heartbeat to a suspended thread must be rejected")
	}
	reg, _ := LoadThreadRegistry(root)
	got := reg.Threads[thr.ThreadID]
	if got.Status != ThreadStatusSuspended {
		t.Errorf("status after rejected heartbeat = %q, want suspended (no revive)", got.Status)
	}
	if !got.LastSeenAt.Equal(before) {
		t.Error("rejected heartbeat must NOT refresh last_seen_at")
	}
}

// ADR-025 test 3b: register bypasses the live fast-path for a suspended record.
func TestRegisterThread_BypassesSuspendedFastPath(t *testing.T) {
	root := t.TempDir()
	thr := mustRegister(t, root, "claude-pantheon", 4245)
	_, _ = SuspendThread(root, thr.ThreadID, nil)

	// Re-register same (agent, pid) with no pinned ThreadID: must NOT reuse the
	// suspended record (that would silently reactivate without restoring state).
	fresh, err := RegisterThread(root, &Thread{AgentID: "claude-pantheon", Surface: "claude", PID: 4245})
	if err != nil {
		t.Fatalf("RegisterThread: %v", err)
	}
	if fresh.ThreadID == thr.ThreadID {
		t.Error("register must mint a fresh thread, not revive the suspended one via fast-path")
	}
	// The suspended record is untouched.
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].Status != ThreadStatusSuspended {
		t.Error("the suspended record must remain suspended")
	}
}

// ADR-025 test 7: prune never removes suspended (it is non-terminal).
func TestPruneClosed_NeverRemovesSuspended(t *testing.T) {
	root := t.TempDir()
	thr := mustRegister(t, root, "claude-pantheon", 4246)
	_, _ = SuspendThread(root, thr.ThreadID, nil)

	reg, _ := LoadThreadRegistry(root)
	// Backdate well past any maxAge.
	reg.Threads[thr.ThreadID].LastSeenAt = time.Now().Add(-720 * time.Hour)
	removed := reg.PruneClosed(time.Now(), time.Hour)
	if removed != 0 {
		t.Errorf("prune removed %d suspended thread(s), want 0", removed)
	}
	if _, ok := reg.Threads[thr.ThreadID]; !ok {
		t.Error("suspended thread must survive prune")
	}
}

// ADR-025 reaper guard: a suspended thread (dead PID by design) must NOT be
// reaped to terminal — that would destroy its recoverable state.
func TestReapDeadThreads_SkipsSuspended(t *testing.T) {
	root := t.TempDir()
	host := "test-host"
	// PID 1 is alive; use a PID that's almost certainly dead to prove the point
	// only matters via the suspended guard. Register with this host.
	thr, err := RegisterThread(root, &Thread{AgentID: "claude-pantheon", Surface: "claude", PID: 999999, Host: host})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = SuspendThread(root, thr.ThreadID, nil)

	reaped, err := ReapDeadThreads(root)
	if err != nil {
		t.Fatalf("ReapDeadThreads: %v", err)
	}
	for _, r := range reaped {
		if r.ThreadID == thr.ThreadID {
			t.Fatal("a suspended thread must never be reaped")
		}
	}
	reg, _ := LoadThreadRegistry(root)
	if reg.Threads[thr.ThreadID].Status != ThreadStatusSuspended {
		t.Errorf("status = %q, want suspended (reaper must skip it)", reg.Threads[thr.ThreadID].Status)
	}
}

// ADR-025 acceptance: SessionStart with a stale active record (no suspend/close)
// transitions it active→suspended in place after a retro sync. The record was
// never terminal, so this heal is legal under ADR-022.
func TestReconcileExits_StaleActiveHealedInPlace(t *testing.T) {
	now := time.Now().UTC()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"thr-stale": {
			ThreadID: "thr-stale", AgentID: "claude-pantheon", Surface: "claude",
			Host: "h1", Status: ThreadStatusActive,
			LastSeenAt: now.Add(-30 * time.Minute), // well past the stale window
		},
	}}
	retro := func(_ *Thread) (*SuspendPayload, bool) {
		return &SuspendPayload{ThothRef: "stele-retro", ResumePrompt: "Pantheon Native App"}, true
	}

	out := ReconcileExits(reg, "h1", "", now, DefaultThreadStaleAfter, retro)
	if len(out) != 1 || out[0].Action != ReconcileSuspendedStale || out[0].ThreadID != "thr-stale" {
		t.Fatalf("outcomes = %+v, want one suspended-stale for thr-stale", out)
	}
	got := reg.Threads["thr-stale"]
	if got.Status != ThreadStatusSuspended {
		t.Errorf("status = %q, want suspended (in-place heal)", got.Status)
	}
	if got.SuspendPayload == nil || got.SuspendPayload.ThothRef != "stele-retro" {
		t.Errorf("retro payload not attached: %+v", got.SuspendPayload)
	}
	if got.ThreadID != "thr-stale" {
		t.Error("stale heal must be in place — same thread id, not a successor")
	}
}

// ADR-025 acceptance: SessionStart with a reaped record + recoverable transcript
// mints a suspended SUCCESSOR carrying reaped_from; the reaped record stays
// reaped (terminal, ADR-022). Repeating reconciliation is idempotent (no second
// successor). No transcript → no successor, an unrecoverable warning instead.
func TestReconcileExits_ReapedMintsSuccessorThenWarns(t *testing.T) {
	now := time.Now().UTC()
	mk := func(id string) *ThreadRegistry {
		return &ThreadRegistry{Threads: map[string]*Thread{
			id: {
				ThreadID: id, AgentID: "claude-pantheon", Surface: "claude",
				Host: "h1", Repo: "/repo", Status: ThreadStatusReaped,
				LastSeenAt: now.Add(-1 * time.Minute), // freshly reaped, within lookback
			},
		}}
	}

	// (a) transcript available → mint a successor.
	reg := mk("thr-reaped")
	withTranscript := func(_ *Thread) (*SuspendPayload, bool) {
		return &SuspendPayload{ThothRef: "stele-xyz"}, true
	}
	out := ReconcileExits(reg, "h1", "", now, DefaultThreadStaleAfter, withTranscript)
	if len(out) != 1 || out[0].Action != ReconcileMintedSuccessor || out[0].SuccessorID == "" {
		t.Fatalf("outcomes = %+v, want one minted-successor with a successor id", out)
	}
	if reg.Threads["thr-reaped"].Status != ThreadStatusReaped {
		t.Error("the reaped record must stay reaped (ADR-022 terminal invariant)")
	}
	succ := reg.Threads[out[0].SuccessorID]
	if succ == nil || succ.Status != ThreadStatusSuspended ||
		succ.SuspendPayload == nil || succ.SuspendPayload.ReapedFrom != "thr-reaped" {
		t.Errorf("successor not minted correctly: %+v", succ)
	}

	// Idempotent: a second reconcile must NOT mint another successor.
	out2 := ReconcileExits(reg, "h1", "", now, DefaultThreadStaleAfter, withTranscript)
	if len(out2) != 0 {
		t.Errorf("second reconcile minted again: %+v (must be idempotent)", out2)
	}

	// (b) no transcript → unrecoverable warning, no successor.
	reg2 := mk("thr-reaped-2")
	noTranscript := func(_ *Thread) (*SuspendPayload, bool) { return nil, false }
	out3 := ReconcileExits(reg2, "h1", "", now, DefaultThreadStaleAfter, noTranscript)
	if len(out3) != 1 || out3[0].Action != ReconcileUnrecoverable {
		t.Fatalf("outcomes = %+v, want one warn-unrecoverable", out3)
	}
	if len(reg2.Threads) != 1 {
		t.Error("no successor may be minted when the transcript is unrecoverable")
	}
}

// ADR-025 retention: suspended state is non-prunable by default but accretes
// unbounded without a bound. PruneStaleSuspended is the opt-in retention — old
// suspends (never resumed) are removed; recent suspends are preserved; terminal
// and live records are untouched.
func TestPruneStaleSuspended(t *testing.T) {
	now := time.Now().UTC()
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"old-susp": {ThreadID: "old-susp", AgentID: "claude-pantheon", Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{SuspendedAt: now.Add(-10 * 24 * time.Hour)}},
		"recent-susp": {ThreadID: "recent-susp", AgentID: "claude-pantheon", Status: ThreadStatusSuspended,
			SuspendPayload: &SuspendPayload{SuspendedAt: now.Add(-1 * time.Hour)}},
		"old-susp-nopayload": {ThreadID: "old-susp-nopayload", AgentID: "claude-pantheon", Status: ThreadStatusSuspended,
			LastSeenAt: now.Add(-10 * 24 * time.Hour)}, // falls back to LastSeenAt
		"live":   {ThreadID: "live", AgentID: "claude-pantheon", Status: ThreadStatusActive, LastSeenAt: now.Add(-10 * 24 * time.Hour)},
		"reaped": {ThreadID: "reaped", AgentID: "claude-pantheon", Status: ThreadStatusReaped, LastSeenAt: now.Add(-10 * 24 * time.Hour)},
	}}

	removed := reg.PruneStaleSuspended(now, SuspendedRetention)
	if removed != 2 {
		t.Errorf("removed %d, want 2 (the two old suspends)", removed)
	}
	if _, ok := reg.Threads["old-susp"]; ok {
		t.Error("old suspended (payload SuspendedAt) must be pruned")
	}
	if _, ok := reg.Threads["old-susp-nopayload"]; ok {
		t.Error("old suspended (LastSeenAt fallback) must be pruned")
	}
	if _, ok := reg.Threads["recent-susp"]; !ok {
		t.Error("recent suspended must be preserved (resume-later guarantee)")
	}
	if _, ok := reg.Threads["live"]; !ok {
		t.Error("live thread must never be touched")
	}
	if _, ok := reg.Threads["reaped"]; !ok {
		t.Error("terminal thread is PruneClosed's job, not PruneStaleSuspended's")
	}
	// retention<=0 is a no-op (never a blanket wipe).
	if n := reg.PruneStaleSuspended(now, 0); n != 0 {
		t.Errorf("retention<=0 removed %d, want 0 (no-op guard)", n)
	}
	// Default PruneClosed must STILL never remove suspended (resume-later holds).
	reg2 := &ThreadRegistry{Threads: map[string]*Thread{
		"s": {ThreadID: "s", AgentID: "a", Status: ThreadStatusSuspended, LastSeenAt: now.Add(-99 * 24 * time.Hour)},
	}}
	if n := reg2.PruneClosed(now, time.Hour); n != 0 {
		t.Errorf("PruneClosed removed %d suspended, want 0 (must stay non-prunable by default)", n)
	}
}

// ADR-025 acceptance: reconciliation is host- and agent-scoped — a stale record on
// another host or for another agent is left untouched (a surface heals only its
// own lineage on its own machine).
func TestReconcileExits_ScopedByHostAndAgent(t *testing.T) {
	now := time.Now().UTC()
	stale := func(id, agent, host string) *Thread {
		return &Thread{ThreadID: id, AgentID: agent, Surface: "claude", Host: host,
			Status: ThreadStatusActive, LastSeenAt: now.Add(-30 * time.Minute)}
	}
	reg := &ThreadRegistry{Threads: map[string]*Thread{
		"mine":        stale("mine", "claude-pantheon", "h1"),
		"other-host":  stale("other-host", "claude-pantheon", "h2"),
		"other-agent": stale("other-agent", "claude-home", "h1"),
	}}
	retro := func(_ *Thread) (*SuspendPayload, bool) { return &SuspendPayload{}, true }

	out := ReconcileExits(reg, "h1", "claude-pantheon", now, DefaultThreadStaleAfter, retro)
	if len(out) != 1 || out[0].ThreadID != "mine" {
		t.Fatalf("outcomes = %+v, want only the local same-agent record healed", out)
	}
	if reg.Threads["other-host"].Status != ThreadStatusActive ||
		reg.Threads["other-agent"].Status != ThreadStatusActive {
		t.Error("out-of-scope records (other host / other agent) must be left untouched")
	}
}
