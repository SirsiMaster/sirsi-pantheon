package routerstore

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func has(e TaskEligibility, sub string) bool {
	for _, r := range e.Reasons {
		if strings.Contains(r, sub) {
			return true
		}
	}
	return false
}

// TaskEligibility explains row conditions used by ClaimTask, not service-side
// caller/session authorization. It must never expose the lease token.
func TestTaskEligibilityExplainsEveryRefusal(t *testing.T) {
	s := newTestStore(t)
	add := func(id, blockedBy, resp string) {
		if err := s.AddTask(Task{Agent: "lane", TaskID: id, Subject: id, BlockedBy: blockedBy, ResponsibleParty: resp}); err != nil {
			t.Fatal(err)
		}
	}
	add("free", "", "")
	add("dep", "", "")
	add("on-task", "dep", "")
	add("on-reason", "waiting for the owner", "")
	add("owners", "", "owner")
	add("leased", "", "")
	add("flaky", "", "")

	if _, err := s.TaskEligibility("lane", "missing"); !errors.Is(err, ErrNotFound) || sentinelName(err) != "ErrNotFound" {
		t.Fatalf("unknown task must be ErrNotFound (wire name), got %v", err)
	}
	e, err := s.TaskEligibility("lane", "free")
	if err != nil || !e.Claimable || !e.Dispatchable || has(e, "would succeed") || !has(e, "not evaluated") {
		t.Fatalf("free task: %+v err=%v", e, err)
	}
	if e, _ = s.TaskEligibility("lane", "on-task"); e.Claimable || e.BlockedByState != "task:pending" || !has(e, "blocked_by") {
		t.Fatalf("task dependency: %+v", e)
	}
	if e, _ = s.TaskEligibility("lane", "on-reason"); e.Claimable || e.BlockedByState != "external-reason" {
		t.Fatalf("external reason: %+v", e)
	}
	if e, _ = s.TaskEligibility("lane", "owners"); !e.Claimable || e.Dispatchable || !has(e, "responsible_party") {
		t.Fatalf("owner-owned task is claimable but not dispatchable: %+v", e)
	}

	lease, err := s.ClaimTask("lane", "leased", "w", "thr-1", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	e, _ = s.TaskEligibility("lane", "leased")
	if e.Claimable || !e.LeaseHeld || e.LeaseExpired || e.ClaimedBy != "w" || e.ThreadID != "thr-1" || !has(e, "live lease") {
		t.Fatalf("live lease: %+v", e)
	}
	if strings.Contains(strings.Join(e.Reasons, " "), lease.Token) {
		t.Fatal("the lease token must never appear in the diagnosis")
	}

	for i := 0; i < MaxRetriesPerItem; i++ {
		l, cerr := s.ClaimTask("lane", "flaky", "w", "thr-1", time.Minute)
		if cerr != nil {
			t.Fatal(cerr)
		}
		_ = s.ReleaseTaskLease("lane", "flaky", l.Token, "fail")
	}
	if e, _ = s.TaskEligibility("lane", "flaky"); e.Claimable || e.Attempts != MaxRetriesPerItem || !has(e, "retry ceiling") {
		t.Fatalf("exhausted retries: %+v", e)
	}
	if e, _ = s.TaskEligibility("lane", "flaky"); !has(e, "task resume-claim") || !has(e, "history-preserving") || has(e, "reset-attempts") {
		t.Fatalf("current-main recovery guidance was lost: %+v", e)
	}
	if _, err := s.ClaimTask("lane", "flaky", "w", "thr-1", time.Minute); err == nil {
		t.Fatal("the diagnosis said not claimable, but ClaimTask succeeded")
	}
	if _, err := s.TaskEligibility("lane", "nope"); err == nil {
		t.Fatal("an unknown task must error")
	}
}

func TestTaskEligibilityReportsOnlyRowScopeWithoutMutatingRows(t *testing.T) {
	for _, tc := range []struct {
		name, status, responsible string
		held                      bool
		attempts                  int
		claimable, dispatchable   bool
	}{
		{name: "free", status: "pending", claimable: true, dispatchable: true},
		{name: "other responsible party", status: "pending", responsible: "owner", claimable: true},
		{name: "blocked", status: "blocked"},
		{name: "done", status: "done"},
		{name: "canceled", status: "canceled"},
		{name: "withdrawn", status: "withdrawn"},
		{name: "held", status: "in-progress", held: true},
		{name: "ceiling", status: "pending", attempts: MaxRetriesPerItem},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			if err := s.AddTask(Task{Agent: "lane", TaskID: "scope", Subject: "inert row fixture", ResponsibleParty: tc.responsible}); err != nil {
				t.Fatal(err)
			}
			token := ""
			if tc.held {
				token = "inert-private-lease-sentinel"
			}
			// Fixture-only setup, never a real claim/session/repair operation.
			if _, err := s.db.Exec(`UPDATE tasks SET status=?,attempts=?,lease_token=? WHERE agent='lane' AND task_id='scope';`, tc.status, tc.attempts, token); err != nil {
				t.Fatal(err)
			}
			type snapshot struct {
				status, token string
				attempts      int
			}
			read := func() snapshot {
				var out snapshot
				if err := s.db.QueryRow(`SELECT status,lease_token,attempts FROM tasks WHERE agent='lane' AND task_id='scope';`).Scan(&out.status, &out.token, &out.attempts); err != nil {
					t.Fatal(err)
				}
				return out
			}
			before := read()
			e, err := s.TaskEligibility("lane", "scope")
			if err != nil || e.Claimable != tc.claimable || e.Dispatchable != tc.dispatchable {
				t.Fatalf("row predicate changed: %+v err=%v", e, err)
			}
			if e.Scope != TaskEligibilityScopeRow || e.CallerEvaluation != TaskEligibilityCallerNotEvaluated || has(e, "would succeed") {
				t.Fatalf("unscoped caller assurance: %+v", e)
			}
			raw, err := json.Marshal(e)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(raw, &wire); err != nil {
				t.Fatal(err)
			}
			if wire["scope"] != "task-row" || wire["caller_evaluation"] != "not-evaluated" {
				t.Fatalf("missing explicit JSON scope: %s", raw)
			}
			if token != "" && strings.Contains(string(raw), token) {
				t.Fatal("lease token exposed")
			}
			if after := read(); after != before {
				t.Fatalf("read-only diagnosis changed row: before=%+v after=%+v", before, after)
			}
		})
	}
}
