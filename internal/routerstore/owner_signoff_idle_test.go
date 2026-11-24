package routerstore

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

// Snapshot the displayed history AND the raw custody fields omitted by Task.
// A refused owner action must not alter the row or create/replace its receipt.
type ownerSignOffState struct {
	Task                                               Task
	Token, Expires, ClaimedBy, ThreadID, FailureReason string
	Attempts                                           int
	Receipt                                            string
	HasReceipt                                         bool
}

func ownerSignOffSnapshot(t *testing.T, s *SQLiteStore, agent, id string) ownerSignOffState {
	t.Helper()
	var state ownerSignOffState
	var err error
	state.Task, err = s.GetTask(agent, id)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT lease_token,lease_expires,claimed_by,thread_id,failure_reason,attempts FROM tasks WHERE agent=? AND task_id=?;`, agent, id).
		Scan(&state.Token, &state.Expires, &state.ClaimedBy, &state.ThreadID, &state.FailureReason, &state.Attempts); err != nil {
		t.Fatal(err)
	}
	state.Receipt, state.HasReceipt, err = s.GetState(receiptKey(agent, id))
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func assertOwnerSignOffUnchanged(t *testing.T, s *SQLiteStore, agent, id string, before ownerSignOffState) {
	t.Helper()
	if after := ownerSignOffSnapshot(t, s, agent, id); !reflect.DeepEqual(before, after) {
		t.Fatalf("owner refusal changed original task/custody/receipt: before=%+v after=%+v", before, after)
	}
}

func TestOwnerSignOffPreservesEveryRetainedCustodyField(t *testing.T) {
	for _, tc := range []struct {
		name, status, token, expires, claimedBy, thread string
		claim                                           bool
	}{
		{name: "live-claim", claim: true},
		{name: "expired-claim", claim: true, expires: "2000-01-01T00:00:00Z"},
		{name: "token-only", status: "pending", token: "retained"},
		{name: "expired-expiry-only", status: "in-progress", expires: "2000-01-01T00:00:00Z"},
		{name: "malformed-expiry-only", status: "in-progress", expires: "not-a-timestamp"},
		{name: "worker-only", status: "blocked", claimedBy: "retained-worker"},
		{name: "thread-only", status: "pending", thread: "retained-thread"},
		{name: "whitespace-token", status: "in-progress", token: " "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTestStore(t)
			if err := s.AddTask(Task{Agent: "lane", TaskID: "original", Subject: "retain original history", Status: "pending",
				Timeline: []TimelineEntry{{Day: "2026-11-24", Owner: "worker", Label: "original history"}}}); err != nil {
				t.Fatal(err)
			}
			if tc.claim {
				if _, err := s.ClaimTask("lane", "original", "worker", "worker-thread", time.Minute); err != nil {
					t.Fatal(err)
				}
				if tc.expires != "" {
					if _, err := s.exec(`UPDATE tasks SET lease_expires=? WHERE agent='lane' AND task_id='original';`, tc.expires); err != nil {
						t.Fatal(err)
					}
				}
			} else {
				if _, err := s.exec(`UPDATE tasks SET status=?,lease_token=?,lease_expires=?,claimed_by=?,thread_id=?,failure_reason='retain original reason' WHERE agent='lane' AND task_id='original';`,
					tc.status, tc.token, tc.expires, tc.claimedBy, tc.thread); err != nil {
					t.Fatal(err)
				}
			}
			before := ownerSignOffSnapshot(t, s, "lane", "original")
			if err := s.CompleteTaskByOwner("lane", "original", "board:fixture@sirsi.ai", "owner must not erase custody"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("retained custody did not refuse: %v", err)
			}
			assertOwnerSignOffUnchanged(t, s, "lane", "original", before)
		})
	}
}

func TestOwnerSignOffPreservesTerminalRecordsAndReceipts(t *testing.T) {
	s := openTestStore(t)
	for _, id := range []string{"done", "canceled", "withdrawn", "successor"} {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CompleteTaskByOwner("lane", "done", "board:fixture@sirsi.ai", "original completion"); err != nil {
		t.Fatal(err)
	}
	if err := s.CancelTask("lane", "canceled", "board:fixture@sirsi.ai", "original cancellation"); err != nil {
		t.Fatal(err)
	}
	if err := s.WithdrawTask("lane", "withdrawn", "lane/successor", "worker@fixture", "original successor"); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"done", "canceled", "withdrawn"} {
		before := ownerSignOffSnapshot(t, s, "lane", id)
		if err := s.CompleteTaskByOwner("lane", id, "board:other@sirsi.ai", "replacement must not overwrite"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("terminal %s: %v", id, err)
		}
		assertOwnerSignOffUnchanged(t, s, "lane", id, before)
	}
}

func TestOwnerSignOffIdleStatusesAndSignOffBeforeClaim(t *testing.T) {
	for _, status := range []string{"pending", "blocked", "in-progress"} {
		t.Run(status, func(t *testing.T) {
			s := openTestStore(t)
			if err := s.AddTask(Task{Agent: "lane", TaskID: "idle", Subject: "reviewed work", Timeline: []TimelineEntry{{Day: "2026-11-24", Owner: "worker", Label: "retained history"}}}); err != nil {
				t.Fatal(err)
			}
			// Model an idle status without manufacturing or clearing any claim.
			if _, err := s.exec(`UPDATE tasks SET status=? WHERE agent='lane' AND task_id='idle';`, status); err != nil {
				t.Fatal(err)
			}
			before := ownerSignOffSnapshot(t, s, "lane", "idle")
			const actor, reason = "board:fixture@sirsi.ai", "explicit owner verification; not a fabricated run receipt"
			if err := s.CompleteTaskByOwner("lane", "idle", actor, reason); err != nil {
				t.Fatal(err)
			}
			after := ownerSignOffSnapshot(t, s, "lane", "idle")
			if after.Task.Status != "done" || after.Task.ResultRef != "owner:"+actor || after.FailureReason != reason || !reflect.DeepEqual(before.Task.Timeline, after.Task.Timeline) {
				t.Fatalf("incorrect owner completion: %+v", after)
			}
			var receipt Receipt
			if err := json.Unmarshal([]byte(after.Receipt), &receipt); err != nil {
				t.Fatal(err)
			}
			if !after.HasReceipt || receipt.Kind != "owner" || receipt.Tier != "owner" || receipt.Actor != actor || receipt.Value != actor || receipt.Reason != reason {
				t.Fatalf("not the exact owner-tier receipt: %+v", receipt)
			}
			if _, err := s.ClaimTask("lane", "idle", "worker", "worker-thread", time.Minute); err == nil {
				t.Fatal("claimed owner-completed task")
			}
			assertOwnerSignOffUnchanged(t, s, "lane", "idle", after)
		})
	}
}

func TestOwnerSignOffServedCustodyFence(t *testing.T) {
	for _, expired := range []bool{false, true} {
		name := "live"
		if expired {
			name = "expired"
		}
		t.Run(name, func(t *testing.T) {
			f := newScopedFixture(t, []string{"router-board", ObserverScope, LedgerEditorScope})
			if err := f.backend.AddTask(Task{Agent: "lane", TaskID: "claimed", Subject: "original worker custody"}); err != nil {
				t.Fatal(err)
			}
			if _, err := f.backend.ClaimTask("lane", "claimed", "worker", "worker-thread", time.Minute); err != nil {
				t.Fatal(err)
			}
			if expired {
				if _, err := f.backend.exec(`UPDATE tasks SET lease_expires='2000-01-01T00:00:00Z' WHERE agent='lane' AND task_id='claimed';`); err != nil {
					t.Fatal(err)
				}
			}
			before := ownerSignOffSnapshot(t, f.backend, "lane", "claimed")
			if err := f.client("router-board").CompleteTaskByOwner("lane", "claimed", "board:fixture@sirsi.ai", "must not override worker"); !errors.Is(err, ErrNotFound) {
				t.Fatalf("served custody refusal: %v", err)
			}
			assertOwnerSignOffUnchanged(t, f.backend, "lane", "claimed", before)
		})
	}
}

func TestOwnerSignOffServedAuthorizationAndIdlePositive(t *testing.T) {
	for _, tc := range []struct {
		name         string
		scope        []string
		agent, actor string
		allow        bool
	}{
		{"editor-idle", []string{"router-board", ObserverScope, LedgerEditorScope}, "router-board", "board:fixture@sirsi.ai", true},
		{"editor-invalid-actor", []string{"router-board", ObserverScope, LedgerEditorScope}, "router-board", "lane", false},
		{"editor-missing-email", []string{"router-board", ObserverScope, LedgerEditorScope}, "router-board", "board:fixture", false},
		{"normal-lane", []string{"lane"}, "lane", "board:fixture@sirsi.ai", false},
		{"observer-only", []string{"router-board", ObserverScope}, "router-board", "board:fixture@sirsi.ai", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newScopedFixture(t, tc.scope)
			if err := f.backend.AddTask(Task{Agent: "lane", TaskID: "idle", Subject: "reviewed idle task"}); err != nil {
				t.Fatal(err)
			}
			before := ownerSignOffSnapshot(t, f.backend, "lane", "idle")
			err := f.client(tc.agent).CompleteTaskByOwner("lane", "idle", tc.actor, "owner verified existing work")
			if !tc.allow {
				wantLaneScope(t, tc.name, err)
				assertOwnerSignOffUnchanged(t, f.backend, "lane", "idle", before)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			after := ownerSignOffSnapshot(t, f.backend, "lane", "idle")
			var receipt Receipt
			if err := json.Unmarshal([]byte(after.Receipt), &receipt); err != nil {
				t.Fatal(err)
			}
			if after.Task.Status != "done" || !after.HasReceipt || receipt.Kind != "owner" || receipt.Tier != "owner" || receipt.Actor != tc.actor {
				t.Fatalf("served idle completion lost owner identity: %+v", after)
			}
		})
	}
}
