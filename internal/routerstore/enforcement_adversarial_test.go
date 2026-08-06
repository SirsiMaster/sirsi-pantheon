package routerstore

import (
	"errors"
	"testing"
	"time"
)

func TestClaimNextUsesSameDependencyPredicateAsRunnable(t *testing.T) {
	s := newTestStore(t)
	blocked, _ := s.Send("owner", "codex-home", "blocked", "review", "wait")
	if ifErr1 := s.SetBlockedBy(blocked, "missing-dependency"); ifErr1 != nil {
		t.Fatal(ifErr1)
	}
	ready, _ := s.Send("owner", "codex-home", "ready", "review", "go")
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.ItemID != ready {
		t.Fatalf("claimed blocked item %s instead of actionable %s", lease.ItemID, ready)
	}
	if ifErr2 := s.Complete(ready, lease.Token, "done"); ifErr2 != nil {
		t.Fatal(ifErr2)
	}
	if _, ifErr3 := s.ClaimNext("codex-home", time.Minute); !errors.Is(ifErr3, ErrNoWork) {
		t.Fatalf("missing dependency must remain unclaimable, got %v", ifErr3)
	}
}

func TestDeadLetterDependencyNeverReleasesClaim(t *testing.T) {
	s := newTestStore(t)
	if ifErr4 := s.MarkRequirementAudit("codex-home", "proof://empty-canon"); ifErr4 != nil {
		t.Fatal(ifErr4)
	}
	dep, _ := s.Send("owner", "codex-home", "dep", "review", "fail")
	blocked, _ := s.Send("owner", "codex-home", "blocked", "review", "wait")
	if ifErr5 := s.SetBlockedBy(blocked, dep); ifErr5 != nil {
		t.Fatal(ifErr5)
	}
	if _, ifErr6 := s.db.Exec(`UPDATE items SET status='dead_letter' WHERE id=?`, dep); ifErr6 != nil {
		t.Fatal(ifErr6)
	}
	if _, ifErr7 := s.ClaimNext("codex-home", time.Minute); !errors.Is(ifErr7, ErrNoWork) {
		t.Fatalf("dead-letter dependency must not release dependent: %v", ifErr7)
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
	if _, ifErr8 := s.ClaimWakeEvent(time.Minute); ifErr8 != nil {
		t.Fatal(ifErr8)
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
	if ifErr9 := tx.Commit(); ifErr9 != nil {
		t.Fatal(ifErr9)
	}
}

func TestReconcileBackfillsWakeForSuccessfullyReleasedDependency(t *testing.T) {
	s := newTestStore(t)
	_ = s.MarkRequirementAudit("codex-home", "proof://empty")
	dep, _ := s.Send("owner", "codex-home", "dep", "review", "first")
	dependent, _ := s.Send("owner", "codex-home", "dependent", "review", "second")
	if ifErr10 := s.SetBlockedBy(dependent, dep); ifErr10 != nil {
		t.Fatal(ifErr10)
	}
	if ifErr11 := s.CloseItem(dep, "done"); ifErr11 != nil {
		t.Fatal(ifErr11)
	}
	if _, ifErr12 := s.db.Exec(`DELETE FROM wake_events WHERE agent='codex-home'`); ifErr12 != nil {
		t.Fatal(ifErr12)
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
	if ifErr13 := s.AddTask(Task{Agent: "codex-home", TaskID: "A", Subject: "first"}); ifErr13 != nil {
		t.Fatal(ifErr13)
	}
	if ifErr14 := s.AddTask(Task{Agent: "codex-home", TaskID: "B", Subject: "second", BlockedBy: "A"}); ifErr14 != nil {
		t.Fatal(ifErr14)
	}
	lease, err := s.ClaimNextTask("codex-home", "worker", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if lease.TaskID != "A" {
		t.Fatalf("claimed %s", lease.TaskID)
	}
	if ifErr15 := s.CompleteTaskLease("codex-home", "A", lease.Token, "proof://A"); ifErr15 != nil {
		t.Fatal(ifErr15)
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
	if _, ifErr16 := s.db.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated) VALUES('lane-wake','lane-wake','codex-home','lane','codex-home','continue',?,?)`, now, now); ifErr16 != nil {
		t.Fatal(ifErr16)
	}
	e, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, ifErr17 := s.Send("owner", "claude-home", "other", "review", "no"); ifErr17 != nil {
		t.Fatal(ifErr17)
	}
	if _, ifErr18 := s.ClaimNext("claude-home", time.Minute); ifErr18 != nil {
		t.Fatal(ifErr18)
	}
	events, _ := s.ListWakeEvents("codex-home")
	if events[0].Status != "leased" {
		t.Fatalf("other lane acknowledged wake: %+v", events[0])
	}
	if _, ifErr19 := s.Send("owner", "codex-home", "mine", "review", "yes"); ifErr19 != nil {
		t.Fatal(ifErr19)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ifErr20 := s.AckWakeEvent(e.EventID, e.LeaseToken, "router-lease:"+lease.ItemID+":"+lease.Token); ifErr20 != nil && !errors.Is(ifErr20, ErrLeaseInvalid) {
		t.Fatal(ifErr20)
	}
	events, _ = s.ListWakeEvents("codex-home")
	if events[0].Status != "acked" {
		t.Fatalf("same-lane real claim did not acknowledge lane wake: %+v", events[0])
	}
}

func TestWaiverRequiresDurableOwnerDecision(t *testing.T) {
	s := newTestStore(t)
	r, _ := s.AddRequirement("x", "ADR-057", "", "codex-home")
	if ifErr21 := s.Waive(r.ID, "skip", "fabricated"); ifErr21 == nil {
		t.Fatal("fabricated authority accepted")
	}
	nonOwner, _ := s.Send("claude-home", "codex-home", "waive", "decision", "no")
	_ = s.CloseItem(nonOwner, "approved")
	if ifErr22 := s.Waive(r.ID, "skip", nonOwner); ifErr22 == nil {
		t.Fatal("non-owner decision accepted")
	}
	owner, _ := s.Send("owner", "codex-home", "waive", "decision", "yes")
	_ = s.CloseItem(owner, "owner approved")
	if ifErr23 := s.Waive(r.ID, "superseded", owner); ifErr23 != nil {
		t.Fatal(ifErr23)
	}
}

func TestAllLeaseEntryPointsRejectUnboundedTTL(t *testing.T) {
	s := newTestStore(t)
	_, _ = s.Send("owner", "codex-home", "item", "review", "x")
	if _, ifErr24 := s.ClaimNext("codex-home", MaxLeaseTTL+time.Second); ifErr24 == nil {
		t.Fatal("item claim accepted unbounded ttl")
	}
	itemLease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ifErr25 := s.RenewLease(itemLease.ItemID, itemLease.Token, MaxLeaseTTL+time.Second); ifErr25 == nil {
		t.Fatal("item renewal accepted unbounded ttl")
	}
	if ifErr26 := s.AddTask(Task{Agent: "codex-home", TaskID: "ttl", Subject: "ttl"}); ifErr26 != nil {
		t.Fatal(ifErr26)
	}
	if _, ifErr27 := s.ClaimNextTask("codex-home", "worker", "thread", MaxLeaseTTL+time.Second); ifErr27 == nil {
		t.Fatal("task claim accepted unbounded ttl")
	}
	taskLease, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ifErr28 := s.RenewTaskLease(taskLease.Agent, taskLease.TaskID, taskLease.Token, MaxLeaseTTL+time.Second); ifErr28 == nil {
		t.Fatal("task renewal accepted unbounded ttl")
	}
}

func TestEscalationDefaultsToOwnerAndIsConfigurable(t *testing.T) {
	s := newTestStore(t)
	tx, err := s.beginImmediate()
	if err != nil {
		t.Fatal(err)
	}
	if ifErr29 := s.escalateTx(tx, s.clock(), "source", "failure", "title", "body"); ifErr29 != nil {
		t.Fatal(ifErr29)
	}
	if ifErr30 := tx.Commit(); ifErr30 != nil {
		t.Fatal(ifErr30)
	}
	var recipient string
	if ifErr31 := s.db.QueryRow(`SELECT to_agent FROM items WHERE source_item='source'`).Scan(&recipient); ifErr31 != nil {
		t.Fatal(ifErr31)
	}
	if recipient != "owner" {
		t.Fatalf("default escalation recipient=%q", recipient)
	}
	s.escalationAgent = "review-board"
	tx, err = s.beginImmediate()
	if err != nil {
		t.Fatal(err)
	}
	if ifErr32 := s.escalateTx(tx, s.clock(), "source-2", "failure", "title", "body"); ifErr32 != nil {
		t.Fatal(ifErr32)
	}
	if ifErr33 := tx.Commit(); ifErr33 != nil {
		t.Fatal(ifErr33)
	}
	if ifErr34 := s.db.QueryRow(`SELECT to_agent FROM items WHERE source_item='source-2'`).Scan(&recipient); ifErr34 != nil {
		t.Fatal(ifErr34)
	}
	if recipient != "review-board" {
		t.Fatalf("configured escalation recipient=%q", recipient)
	}
}
