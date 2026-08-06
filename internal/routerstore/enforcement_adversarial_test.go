package routerstore

import (
	"errors"
	"testing"
	"time"
)

func TestClaimNextUsesSameDependencyPredicateAsRunnable(t *testing.T) {
	s := newTestStore(t)
	blocked, _ := s.Send("owner", "codex-home", "blocked", "review", "wait")
	if err := s.SetBlockedBy(blocked, "missing-dependency"); err != nil {
		t.Fatal(err)
	}
	ready, _ := s.Send("owner", "codex-home", "ready", "review", "go")
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.ItemID != ready {
		t.Fatalf("claimed blocked item %s instead of actionable %s", lease.ItemID, ready)
	}
	if err := s.Complete(ready, lease.Token, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNext("codex-home", time.Minute); !errors.Is(err, ErrNoWork) {
		t.Fatalf("missing dependency must remain unclaimable, got %v", err)
	}
}

func TestDeadLetterDependencyNeverReleasesClaim(t *testing.T) {
	s := newTestStore(t)
	if err := s.MarkRequirementAudit("codex-home", "proof://empty-canon"); err != nil {
		t.Fatal(err)
	}
	dep, _ := s.Send("owner", "codex-home", "dep", "review", "fail")
	blocked, _ := s.Send("owner", "codex-home", "blocked", "review", "wait")
	if err := s.SetBlockedBy(blocked, dep); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE items SET status='dead_letter' WHERE id=?`, dep); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNext("codex-home", time.Minute); !errors.Is(err, ErrNoWork) {
		t.Fatalf("dead-letter dependency must not release dependent: %v", err)
	}
	state, err := s.ClassifyLane("codex-home", true, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if state.Classification != LaneBlocked || state.BlockedWork == 0 {
		t.Fatalf("dead-letter dependency must remain visibly BLOCKED: %+v", state)
	}
}

func TestGlobalWakeExpiryPathRetriesWithoutSQLError(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	_, _ = s.Send("owner", "codex-home", "x", "review", "x")
	if _, err := s.ClaimWakeEvent(time.Minute); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	tx, err := s.beginImmediate()
	if err != nil {
		t.Fatal(err)
	}
	res, err := expireWakeLeasesTx(tx, now, "")
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("expired %d wakes", n)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileBackfillsWakeForSuccessfullyReleasedDependency(t *testing.T) {
	s := newTestStore(t)
	_ = s.MarkRequirementAudit("codex-home", "proof://empty")
	dep, _ := s.Send("owner", "codex-home", "dep", "review", "first")
	dependent, _ := s.Send("owner", "codex-home", "dependent", "review", "second")
	if err := s.SetBlockedBy(dependent, dep); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseItem(dep, "done"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`DELETE FROM wake_events WHERE agent='codex-home'`); err != nil {
		t.Fatal(err)
	}
	report, err := s.ReconcileOperationalState("codex-home", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.WakeEventsCreated != 1 {
		t.Fatalf("runnable released dependency got %d backfill wakes", report.WakeEventsCreated)
	}
}

func TestCompletedTaskDependencyBecomesRunnableClaimableAndWoken(t *testing.T) {
	s := newTestStore(t)
	_ = s.MarkRequirementAudit("codex-home", "proof://empty")
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "A", Subject: "first"}); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "B", Subject: "second", BlockedBy: "A"}); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNextTask("codex-home", "worker", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.TaskID != "A" {
		t.Fatalf("claimed %s", lease.TaskID)
	}
	if err := s.CompleteTaskLease("codex-home", "A", lease.Token, "proof://A"); err != nil {
		t.Fatal(err)
	}
	r, err := s.RunnableFor("codex-home")
	if err != nil {
		t.Fatal(err)
	}
	if r.ActionableLedgerTasks != 1 {
		t.Fatalf("completed dependency did not release B: %+v", r)
	}
	lease, err = s.ClaimNextTask("codex-home", "worker", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.TaskID != "B" {
		t.Fatalf("released task not claimable: %+v", lease)
	}
	events, _ := s.ListWakeEvents("codex-home")
	found := false
	for _, e := range events {
		found = found || (e.SourceKind == "ledger_task" && e.SourceID == "B" && e.Reason == "ledger task dependency completed successfully")
	}
	if !found {
		t.Fatalf("completed task dependency emitted no dependent wake: %+v", events)
	}
}

func TestLaneWakeAcknowledgedOnlyBySameLaneRealClaim(t *testing.T) {
	s := newTestStore(t)
	now := s.clock().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated) VALUES('lane-wake','lane-wake','codex-home','lane','codex-home','continue',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}
	e, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send("owner", "claude-home", "other", "review", "no"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNext("claude-home", time.Minute); err != nil {
		t.Fatal(err)
	}
	events, _ := s.ListWakeEvents("codex-home")
	if events[0].Status != "leased" {
		t.Fatalf("other lane acknowledged wake: %+v", events[0])
	}
	if _, err := s.Send("owner", "codex-home", "mine", "review", "yes"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AckWakeEvent(e.EventID, e.LeaseToken, "router-lease:"+lease.ItemID+":"+lease.Token); err != nil && !errors.Is(err, ErrLeaseInvalid) {
		t.Fatal(err)
	}
	events, _ = s.ListWakeEvents("codex-home")
	if events[0].Status != "acked" {
		t.Fatalf("same-lane real claim did not acknowledge lane wake: %+v", events[0])
	}
}

func TestWaiverRequiresDurableOwnerDecision(t *testing.T) {
	s := newTestStore(t)
	r, _ := s.AddRequirement("x", "ADR-057", "", "codex-home")
	if err := s.Waive(r.ID, "skip", "fabricated"); err == nil {
		t.Fatal("fabricated authority accepted")
	}
	nonOwner, _ := s.Send("claude-home", "codex-home", "waive", "decision", "no")
	_ = s.CloseItem(nonOwner, "approved")
	if err := s.Waive(r.ID, "skip", nonOwner); err == nil {
		t.Fatal("non-owner decision accepted")
	}
	owner, _ := s.Send("owner", "codex-home", "waive", "decision", "yes")
	_ = s.CloseItem(owner, "owner approved")
	if err := s.Waive(r.ID, "superseded", owner); err != nil {
		t.Fatal(err)
	}
}

func TestAllLeaseEntryPointsRejectUnboundedTTL(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Send("owner", "codex-home", "item", "review", "x")
	if _, err := s.ClaimNext("codex-home", MaxLeaseTTL+time.Second); err == nil {
		t.Fatal("item claim accepted unbounded ttl")
	}
	itemLease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RenewLease(itemLease.ItemID, itemLease.Token, MaxLeaseTTL+time.Second); err == nil {
		t.Fatal("item renewal accepted unbounded ttl")
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "ttl", Subject: "ttl"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNextTask("codex-home", "worker", "thread", MaxLeaseTTL+time.Second); err == nil {
		t.Fatal("task claim accepted unbounded ttl")
	}
	taskLease, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.RenewTaskLease(taskLease.Agent, taskLease.TaskID, taskLease.Token, MaxLeaseTTL+time.Second); err == nil {
		t.Fatal("task renewal accepted unbounded ttl")
	}
}

func TestEscalationDefaultsToOwnerAndIsConfigurable(t *testing.T) {
	s := newTestStore(t)
	tx, err := s.beginImmediate()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.escalateTx(tx, s.clock(), "source", "failure", "title", "body"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var recipient string
	if err := s.db.QueryRow(`SELECT to_agent FROM items WHERE source_item='source'`).Scan(&recipient); err != nil {
		t.Fatal(err)
	}
	if recipient != "owner" {
		t.Fatalf("default escalation recipient=%q", recipient)
	}
	s.escalationAgent = "review-board"
	tx, err = s.beginImmediate()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.escalateTx(tx, s.clock(), "source-2", "failure", "title", "body"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT to_agent FROM items WHERE source_item='source-2'`).Scan(&recipient); err != nil {
		t.Fatal(err)
	}
	if recipient != "review-board" {
		t.Fatalf("configured escalation recipient=%q", recipient)
	}
}
