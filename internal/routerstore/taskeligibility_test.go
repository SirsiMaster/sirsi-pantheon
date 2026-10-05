package routerstore

import (
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

// TaskEligibility must agree with what ClaimTask actually does, for every
// refusal cause, and must never expose the lease token (both directions).
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

	e, err := s.TaskEligibility("lane", "free")
	if err != nil || !e.Claimable || !e.Dispatchable || !has(e, "would succeed") {
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
	if _, err := s.ClaimTask("lane", "flaky", "w", "thr-1", time.Minute); err == nil {
		t.Fatal("the diagnosis said not claimable, but ClaimTask succeeded")
	}
	if _, err := s.TaskEligibility("lane", "nope"); err == nil {
		t.Fatal("an unknown task must error")
	}
}
