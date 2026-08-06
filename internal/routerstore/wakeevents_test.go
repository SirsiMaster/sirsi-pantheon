package routerstore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMigration11ReplacesAgentWideWakeAckTriggers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "router.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the unsafe v10 trigger and make the file look like an already
	// deployed v10 database. Reopen must apply the corrective migration.
	_, err = s.db.Exec(`DROP TRIGGER ack_wake_on_item_claim;
		DROP INDEX idx_items_lease_updated;
		ALTER TABLE items DROP COLUMN lease_updated;
		ALTER TABLE requirements DROP COLUMN waiver_ref;
		CREATE TRIGGER ack_wake_on_item_claim AFTER UPDATE OF lease_token ON items
		WHEN NEW.lease_token<>'' AND OLD.lease_token='' BEGIN
		UPDATE wake_events SET status='acked' WHERE agent=NEW.to_agent AND status='leased'; END;
		PRAGMA user_version=10;`)
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	var triggerSQL string
	if err := s.db.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='ack_wake_on_item_claim';`).Scan(&triggerSQL); err != nil {
		t.Fatal(err)
	}
	if version < 11 || !strings.Contains(triggerSQL, "source_id=NEW.id") {
		t.Fatalf("v11 correlation migration missing: version=%d trigger=%s", version, triggerSQL)
	}
}

func TestSourceTransitionsEmitDurableWakeEvents(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "work"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "codex-home", TaskID: "T", Subject: "task"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRequirement("gap", "ADR-057", "R3", "codex-home"); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListWakeEvents("codex-home")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("want item+task+requirement wakes, got %d: %+v", len(events), events)
	}
}

func TestWakeClaimRequiresStoreAckAndFences(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send("owner", "codex-home", "item", "review", "work"); err != nil {
		t.Fatal(err)
	}
	e, err := s.ClaimWakeEvent(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AckWakeEvent(e.EventID, e.LeaseToken, " "); err == nil {
		t.Fatal("heartbeat-like empty ack accepted")
	}
	if err := s.AckWakeEvent(e.EventID, e.LeaseToken, "banana"); err == nil {
		t.Fatal("invented acknowledgment reference accepted")
	}
	if _, err := s.ClaimNext("codex-home", time.Minute); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListWakeEvents("codex-home")
	if err != nil {
		t.Fatal(err)
	}
	ackRef := events[0].AckRef
	if ackRef == "" {
		t.Fatal("exact item claim did not acknowledge event")
	}
	if err := s.AckWakeEvent(e.EventID, "wrong", ackRef); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("wrong token not fenced: %v", err)
	}
}

func TestClaimingDifferentItemCannotAcknowledgeLeasedWake(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Send("owner", "codex-home", "a", "review", "a")
	b, _ := s.Send("owner", "codex-home", "b", "review", "b")
	events, _ := s.ListWakeEvents("codex-home")
	var eventA WakeEvent
	for _, event := range events {
		if event.SourceID == a {
			eventA = event
		}
	}
	// Lease A's event, then make B the only claimable item.
	if _, err := s.db.Exec(`UPDATE wake_events SET created='2020-01-01T00:00:00Z' WHERE event_id=?`, eventA.EventID); err != nil {
		t.Fatal(err)
	}
	leased, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil || leased.SourceID != a {
		t.Fatalf("lease event A: %+v %v", leased, err)
	}
	if _, err := s.db.Exec(`UPDATE items SET status='blocked' WHERE id=?`, a); err != nil {
		t.Fatal(err)
	}
	claim, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil || claim.ItemID != b {
		t.Fatalf("claim B: %+v %v", claim, err)
	}
	wrongRef := "router-lease:" + b + ":" + claim.Token
	if err := s.AckWakeEvent(leased.EventID, leased.LeaseToken, wrongRef); err == nil {
		t.Fatal("explicit claim reference for B acknowledged event A")
	}
	events, _ = s.ListWakeEvents("codex-home")
	for _, event := range events {
		if event.EventID == leased.EventID && event.Status != "leased" {
			t.Fatalf("claiming B falsely acknowledged event A: %+v", event)
		}
	}
}

func TestClaimingDifferentTaskCannotAcknowledgeLeasedWake(t *testing.T) {
	s := newTestStore(t)
	_ = s.AddTask(Task{Agent: "codex-home", TaskID: "A", Subject: "a"})
	_ = s.AddTask(Task{Agent: "codex-home", TaskID: "B", Subject: "b"})
	events, _ := s.ListWakeEvents("codex-home")
	var eventA WakeEvent
	for _, event := range events {
		if event.SourceID == "A" {
			eventA = event
		}
	}
	_, _ = s.db.Exec(`UPDATE wake_events SET created='2020-01-01T00:00:00Z' WHERE event_id=?`, eventA.EventID)
	leased, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil || leased.SourceID != "A" {
		t.Fatalf("lease event A: %+v %v", leased, err)
	}
	_, _ = s.UpdateTask("codex-home", "A", TaskUpdate{BlockedBy: "hold", BlockedBySet: true})
	claim, err := s.ClaimNextTask("codex-home", "worker", "thread", time.Minute)
	if err != nil || claim.TaskID != "B" {
		t.Fatalf("claim B: %+v %v", claim, err)
	}
	wrongRef := "task-lease:codex-home:B:" + claim.Token
	if err := s.AckWakeEvent(leased.EventID, leased.LeaseToken, wrongRef); err == nil {
		t.Fatal("explicit task lease reference for B acknowledged event A")
	}
	events, _ = s.ListWakeEvents("codex-home")
	for _, event := range events {
		if event.EventID == leased.EventID && event.Status != "leased" {
			t.Fatalf("claiming B falsely acknowledged event A: %+v", event)
		}
	}
}

func TestWorkerLeaseAcknowledgesWakeThroughStoreMutation(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "work"); err != nil {
		t.Fatal(err)
	}
	e, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimNext("codex-home", time.Minute); err != nil {
		t.Fatal(err)
	}
	events, _ := s.ListWakeEvents("codex-home")
	if len(events) != 1 || events[0].Status != "acked" || events[0].AckRef == "" {
		t.Fatalf("worker claim did not acknowledge wake: claimed=%+v events=%+v", e, events)
	}
}

func TestWakeRetriesBoundAndTerminallyFail(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if _, err := s.Send("owner", "codex-home", "item", "review", "work"); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= MaxWakeAttempts; attempt++ {
		e, err := s.ClaimWakeEvent(time.Minute)
		if err != nil {
			t.Fatalf("claim %d: %v", attempt, err)
		}
		if err := s.FailWakeEvent(e.EventID, e.LeaseToken, "unroutable"); err != nil {
			t.Fatalf("fail %d: %v", attempt, err)
		}
		now = now.Add(time.Duration(1<<uint(attempt-1))*time.Minute + time.Second)
	}
	events, _ := s.ListWakeEvents("codex-home")
	if len(events) != 1 || events[0].Status != "terminal_failed" || events[0].Attempts != MaxWakeAttempts {
		t.Fatalf("wake did not converge to terminal failure: %+v", events)
	}
}

func TestWakeLeaseExpiryWithoutStoreActionTerminallyFails(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := s.MarkRequirementAudit("codex-home", "audit://current"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send("owner", "codex-home", "item", "review", "work"); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= MaxWakeAttempts; attempt++ {
		if _, err := s.ClaimWakeEventFor("codex-home", time.Minute); err != nil {
			t.Fatalf("claim %d: %v", attempt, err)
		}
		// Invocation returned success, but the worker never mutated the store.
		now = now.Add(time.Minute + time.Second)
		if _, err := s.ReconcileOperationalState("codex-home", true); err != nil {
			t.Fatalf("expire silent delivery %d: %v", attempt, err)
		}
	}
	events, _ := s.ListWakeEvents("codex-home")
	if len(events) != 1 || events[0].Status != "terminal_failed" || events[0].Attempts != MaxWakeAttempts {
		t.Fatalf("silent wake expiries did not terminate: %+v", events)
	}
}

func TestCompletionEmitsContinueWhenMoreWorkExists(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Send("owner", "codex-home", "a", "review", "a")
	if _, err := s.Send("owner", "codex-home", "b", "review", "b"); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseItem(a, "done"); err != nil {
		t.Fatal(err)
	}
	events, _ := s.ListWakeEvents("codex-home")
	found := false
	for _, e := range events {
		found = found || e.Reason == "worker completed item while more work exists"
	}
	if !found {
		t.Fatalf("completion with remaining work emitted no continuation wake: %+v", events)
	}
}

func TestTerminalDependencyWakesDependentTarget(t *testing.T) {
	s := newTestStore(t)
	dependency, _ := s.Send("owner", "claude-home", "dependency", "review", "first")
	dependent, _ := s.Send("owner", "codex-home", "dependent", "review", "second")
	if err := s.SetBlockedBy(dependent, dependency); err != nil {
		t.Fatal(err)
	}
	if err := s.CloseItem(dependency, "resolved"); err != nil {
		t.Fatal(err)
	}
	events, _ := s.ListWakeEvents("codex-home")
	found := false
	for _, e := range events {
		found = found || (e.SourceID == dependent && e.Reason == "inbox dependency completed successfully")
	}
	if !found {
		t.Fatalf("terminal dependency did not wake dependent target: %+v", events)
	}
}
