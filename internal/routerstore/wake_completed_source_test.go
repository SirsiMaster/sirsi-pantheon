package routerstore

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

// Reproduce the pre-fix durable residue only in a disposable fixture: a real
// fenced completed task whose source wake never received the claim ACK.
func legacyDoneWake(t *testing.T, s *SQLiteStore, status string) WakeEvent {
	t.Helper()
	if err := s.AddTask(Task{Agent: "lane", TaskID: "old", Subject: "old real work"}); err != nil {
		t.Fatal(err)
	}
	l, err := s.ClaimTask("lane", "old", "w", "thr", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteTaskLease("lane", "old", l.Token, "canonical://real-old-result"); err != nil {
		t.Fatal(err)
	}
	expires := s.clock().UTC().Add(-time.Minute).Format(time.RFC3339)
	if _, err = s.db.Exec(`UPDATE wake_events SET status=?,attempts=3,lease_token='fixture-wake',lease_expires=?,ack_ref='',last_error='old delivery',created='2020-01-01T00:00:00Z',updated='2020-01-01T00:00:00Z' WHERE agent='lane' AND source_kind='ledger_task' AND source_id='old'`, status, expires); err != nil {
		t.Fatal(err)
	}
	return exactOldWake(t, s)
}
func exactOldWake(t *testing.T, s *SQLiteStore) WakeEvent {
	t.Helper()
	events, err := s.ListWakeEvents("lane")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.SourceKind == "ledger_task" && e.SourceID == "old" {
			return e
		}
	}
	t.Fatal("missing old source wake")
	return WakeEvent{}
}
func TestCompletedTaskWakePendingAndExpiredLeaseAreNonExecuting(t *testing.T) {
	for _, status := range []string{"pending", "leased"} {
		t.Run(status, func(t *testing.T) {
			s := newTestStore(t)
			before := legacyDoneWake(t, s, status)
			if _, err := s.ClaimWakeEventFor("lane", time.Minute); !errors.Is(err, ErrNoWork) {
				t.Fatalf("done source selected: %v", err)
			}
			tx, err := s.beginImmediate()
			if err != nil {
				t.Fatal(err)
			}
			res, err := expireWakeLeasesTx(tx, s.clock().UTC(), "")
			if err != nil {
				tx.Rollback()
				t.Fatal(err)
			}
			n, err := res.RowsAffected()
			if err != nil || n != 0 {
				tx.Rollback()
				t.Fatal("done source expired into retry/failure")
			}
			if err = tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if after := exactOldWake(t, s); !reflect.DeepEqual(before, after) {
				t.Fatal("nonexecuting exclusion changed evidence")
			}
		})
	}
}
func TestCompletedTaskWakeCannotBeFailedAfterCompletion(t *testing.T) {
	s := newTestStore(t)
	before := legacyDoneWake(t, s, "leased")
	// Make lease live so the completed-source guard, not expiry, refuses failure.
	if _, err := s.db.Exec(`UPDATE wake_events SET lease_expires=? WHERE event_id=?`, s.clock().UTC().Add(time.Minute).Format(time.RFC3339), before.EventID); err != nil {
		t.Fatal(err)
	}
	before = exactOldWake(t, s)
	if err := s.FailWakeEvent(before.EventID, "fixture-wake", "adapter error after source completed"); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("completed source failed: %v", err)
	}
	if !reflect.DeepEqual(before, exactOldWake(t, s)) {
		t.Fatal("completed source failure mutated evidence")
	}
}
func TestCompletedTaskWakeDoesNotSuppressRunnableContinuation(t *testing.T) {
	s := newTestStore(t)
	before := legacyDoneWake(t, s, "pending")
	if err := s.MarkRequirementAudit("lane", "audit://current"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddTask(Task{Agent: "lane", TaskID: "new", Subject: "real new work"}); err != nil {
		t.Fatal(err)
	}
	// Simulate an earlier acknowledged worker that vanished before claiming new.
	if _, err := s.db.Exec(`UPDATE wake_events SET status='acked',ack_ref='fixture-previous-delivery' WHERE agent='lane' AND source_id='new' AND source_kind='ledger_task'`); err != nil {
		t.Fatal(err)
	}
	report, err := s.ReconcileOperationalState("lane", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.WakeEventsCreated != 1 {
		t.Fatalf("obsolete done wake suppressed continuation: %d", report.WakeEventsCreated)
	}
	if !reflect.DeepEqual(before, exactOldWake(t, s)) {
		t.Fatal("reconcile changed old evidence")
	}
	event, err := s.ClaimWakeEventFor("lane", time.Minute)
	if err != nil || event.SourceKind != "lane" {
		t.Fatalf("did not select real continuation: %v", err)
	}
}
func TestIncompleteOrWrongLaneTaskWakeIsNotExcluded(t *testing.T) {
	for _, mode := range []string{"empty-result", "no-attempt", "reopened", "wrong-lane", "wrong-kind", "missing-task", "unmet-requirement"} {
		t.Run(mode, func(t *testing.T) {
			s := newTestStore(t)
			w := legacyDoneWake(t, s, "pending")
			var err error
			switch mode {
			case "empty-result":
				_, err = s.db.Exec(`UPDATE tasks SET result_ref='' WHERE agent='lane' AND task_id='old'`)
			case "no-attempt":
				_, err = s.db.Exec(`UPDATE tasks SET attempts=0 WHERE agent='lane' AND task_id='old'`)
			case "reopened":
				_, err = s.db.Exec(`UPDATE tasks SET status='pending' WHERE agent='lane' AND task_id='old'`)
			case "wrong-lane":
				_, err = s.db.Exec(`UPDATE wake_events SET agent='other' WHERE event_id=?`, w.EventID)
			case "wrong-kind":
				_, err = s.db.Exec(`UPDATE wake_events SET source_kind='router_item' WHERE event_id=?`, w.EventID)
			case "missing-task":
				_, err = s.db.Exec(`UPDATE wake_events SET source_id='missing' WHERE event_id=?`, w.EventID)
			case "unmet-requirement":
				req, e := s.AddRequirement("unmet", "ADR-061", "R1", "lane")
				if e != nil {
					t.Fatal(e)
				}
				_, err = s.db.Exec(`UPDATE tasks SET task_id=? WHERE agent='lane' AND task_id='old'`, "requirement/"+req.ID)
				if err == nil {
					_, err = s.db.Exec(`UPDATE wake_events SET source_id=? WHERE event_id=?`, "requirement/"+req.ID, w.EventID)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			agent := "lane"
			if mode == "wrong-lane" {
				agent = "other"
			}
			event, err := s.ClaimWakeEventFor(agent, time.Minute)
			if err != nil || event == nil || event.EventID != w.EventID {
				t.Fatalf("unproven done source suppressed: %v", err)
			}
		})
	}
}
