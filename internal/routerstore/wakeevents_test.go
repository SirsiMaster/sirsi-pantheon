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
	s, err := OpenPath(path)
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
	s, err = OpenPath(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	var triggerSQL string
	if ifErr1 := s.db.QueryRow(`PRAGMA user_version;`).Scan(&version); ifErr1 != nil {
		t.Fatal(ifErr1)
	}
	if ifErr2 := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='trigger' AND name='ack_wake_on_item_claim';`).Scan(&triggerSQL); ifErr2 != nil {
		t.Fatal(ifErr2)
	}
	if version < 11 || !strings.Contains(triggerSQL, "source_id=NEW.id") {
		t.Fatalf("v11 correlation migration missing: version=%d trigger=%s", version, triggerSQL)
	}
}

func TestSourceTransitionsEmitDurableWakeEvents(t *testing.T) {
	s := newTestStore(t)
	if _, ifErr3 := s.Send("owner", "codex-home", "item", "review", "work"); ifErr3 != nil {
		t.Fatal(ifErr3)
	}
	if ifErr4 := s.AddTask(Task{Agent: "codex-home", TaskID: "T", Subject: "task"}); ifErr4 != nil {
		t.Fatal(ifErr4)
	}
	if _, ifErr5 := s.AddRequirement("gap", "ADR-061", "R3", "codex-home"); ifErr5 != nil {
		t.Fatal(ifErr5)
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
	if ifErr6 := s.MarkRequirementAudit("codex-home", "audit://current"); ifErr6 != nil {
		t.Fatal(ifErr6)
	}
	if _, ifErr7 := s.Send("owner", "codex-home", "item", "review", "work"); ifErr7 != nil {
		t.Fatal(ifErr7)
	}
	e, err := s.ClaimWakeEvent(time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if ifErr8 := s.AckWakeEvent(e.EventID, e.LeaseToken, " "); ifErr8 == nil {
		t.Fatal("heartbeat-like empty ack accepted")
	}
	if ifErr9 := s.AckWakeEvent(e.EventID, e.LeaseToken, "banana"); ifErr9 == nil {
		t.Fatal("invented acknowledgment reference accepted")
	}
	if _, ifErr10 := s.ClaimNext("codex-home", time.Minute); ifErr10 != nil {
		t.Fatal(ifErr10)
	}
	events, err := s.ListWakeEvents("codex-home")
	if err != nil {
		t.Fatal(err)
	}
	ackRef := events[0].AckRef
	if ackRef == "" {
		t.Fatal("exact item claim did not acknowledge event")
	}
	if ifErr11 := s.AckWakeEvent(e.EventID, "wrong", ackRef); !errors.Is(ifErr11, ErrLeaseInvalid) {
		t.Fatalf("wrong token not fenced: %v", ifErr11)
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
	if _, ifErr12 := s.db.Exec(`UPDATE wake_events SET created='2020-01-01T00:00:00Z' WHERE event_id=?`, eventA.EventID); ifErr12 != nil {
		t.Fatal(ifErr12)
	}
	leased, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil || leased.SourceID != a {
		t.Fatalf("lease event A: %+v %v", leased, err)
	}
	if _, ifErr13 := s.db.Exec(`UPDATE items SET status='blocked' WHERE id=?`, a); ifErr13 != nil {
		t.Fatal(ifErr13)
	}
	claim, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil || claim.ItemID != b {
		t.Fatalf("claim B: %+v %v", claim, err)
	}
	wrongRef := "router-lease:" + b + ":" + claim.Token
	if ifErr14 := s.AckWakeEvent(leased.EventID, leased.LeaseToken, wrongRef); ifErr14 == nil {
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
	if ifErr15 := s.AckWakeEvent(leased.EventID, leased.LeaseToken, wrongRef); ifErr15 == nil {
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
	if _, ifErr16 := s.Send("owner", "codex-home", "item", "review", "work"); ifErr16 != nil {
		t.Fatal(ifErr16)
	}
	e, err := s.ClaimWakeEventFor("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, ifErr17 := s.ClaimNext("codex-home", time.Minute); ifErr17 != nil {
		t.Fatal(ifErr17)
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
	if _, ifErr18 := s.Send("owner", "codex-home", "item", "review", "work"); ifErr18 != nil {
		t.Fatal(ifErr18)
	}
	for attempt := 1; attempt <= MaxWakeAttempts; attempt++ {
		e, err := s.ClaimWakeEvent(time.Minute)
		if err != nil {
			t.Fatalf("claim %d: %v", attempt, err)
		}
		if ifErr19 := s.FailWakeEvent(e.EventID, e.LeaseToken, "unroutable"); ifErr19 != nil {
			t.Fatalf("fail %d: %v", attempt, ifErr19)
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
	if ifErr20 := s.MarkRequirementAudit("codex-home", "audit://current"); ifErr20 != nil {
		t.Fatal(ifErr20)
	}
	if _, ifErr21 := s.Send("owner", "codex-home", "item", "review", "work"); ifErr21 != nil {
		t.Fatal(ifErr21)
	}
	for attempt := 1; attempt <= MaxWakeAttempts; attempt++ {
		if _, ifErr22 := s.ClaimWakeEventFor("codex-home", time.Minute); ifErr22 != nil {
			t.Fatalf("claim %d: %v", attempt, ifErr22)
		}
		// Invocation returned success, but the worker never mutated the store.
		now = now.Add(time.Minute + time.Second)
		if _, ifErr23 := s.ReconcileOperationalState("codex-home", true); ifErr23 != nil {
			t.Fatalf("expire silent delivery %d: %v", attempt, ifErr23)
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
	if _, ifErr24 := s.Send("owner", "codex-home", "b", "review", "b"); ifErr24 != nil {
		t.Fatal(ifErr24)
	}
	if ifErr25 := s.CloseItem(a, "done"); ifErr25 != nil {
		t.Fatal(ifErr25)
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
	if ifErr26 := s.SetBlockedBy(dependent, dependency); ifErr26 != nil {
		t.Fatal(ifErr26)
	}
	if ifErr27 := s.CloseItem(dependency, "resolved"); ifErr27 != nil {
		t.Fatal(ifErr27)
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
