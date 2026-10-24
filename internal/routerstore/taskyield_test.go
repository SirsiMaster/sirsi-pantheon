package routerstore

import (
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTaskYieldResumesWithoutFailureExhaustion(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "ra", TaskID: "original", Subject: "phased work"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		l, err := s.ClaimTask("ra", "original", "w", "thread", time.Minute)
		if err != nil {
			t.Fatal(i, err)
		}
		if err = s.YieldTaskLease("ra", "original", l.Token, "clean", "receipt:partial", ""); err != nil {
			t.Fatal(err)
		}
		e, err := s.TaskEligibility("ra", "original")
		if err != nil || e.Attempts != 0 || e.LeaseHeld || !e.Claimable {
			t.Fatal(i, e, err)
		}
		if err = s.YieldTaskLease("ra", "original", l.Token, "clean", "receipt:replay", ""); !errors.Is(err, ErrLeaseInvalid) {
			t.Fatal("replay", err)
		}
	}
	events, err := s.TaskLeaseEventsSince("ra", "original", "2020-01-01T00:00:00Z")
	if err != nil || len(events) != 10 {
		t.Fatal("audit", len(events), err)
	}
	l, err := s.ClaimTask("ra", "original", "w", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CompleteTaskLease("ra", "original", l.Token, "actual-final-proof"); err != nil {
		t.Fatal(err)
	}
}
func TestTaskYieldRetainsFailuresAndDependencies(t *testing.T) {
	s := newTestStore(t)
	_ = s.AddTask(Task{Agent: "ra", TaskID: "t", Subject: "work"})
	l, _ := s.ClaimTask("ra", "t", "w", "thread", time.Minute)
	if err := s.ReleaseTaskLease("ra", "t", l.Token, "real failure"); err != nil {
		t.Fatal(err)
	}
	l, _ = s.ClaimTask("ra", "t", "w", "thread", time.Minute)
	if err := s.YieldTaskLease("ra", "t", l.Token, "external-wait", "receipt:wait", "external:qualification"); err != nil {
		t.Fatal(err)
	}
	e, _ := s.TaskEligibility("ra", "t")
	if e.Attempts != 1 || e.FailureReason != "real failure" || e.Claimable || e.BlockedBy != "external:qualification" {
		t.Fatal(e)
	}
	// Owner clears only the resolved own wait through the existing metadata API.
	if _, err := s.UpdateTask("ra", "t", TaskUpdate{BlockedBySet: true}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		l, err := s.ClaimTask("ra", "t", "w", "thread", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ReleaseTaskLease("ra", "t", l.Token, "true failure"); err != nil {
			t.Fatal(err)
		}
	}
	e, _ = s.TaskEligibility("ra", "t")
	if e.Attempts != 3 || e.Status != "blocked" || e.Claimable {
		t.Fatal(e)
	}
	if err := s.YieldTaskLease("ra", "t", l.Token, "clean", "receipt:fake", ""); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatal(err)
	}
}
func TestTaskYieldConcurrentAndExpiredFences(t *testing.T) {
	s := newTestStore(t)
	_ = s.AddTask(Task{Agent: "ra", TaskID: "t", Subject: "work"})
	l, _ := s.ClaimTask("ra", "t", "w", "thread", time.Minute)
	if err := s.YieldTaskLease("foreign", "t", l.Token, "clean", "ref", ""); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- s.YieldTaskLease("ra", "t", l.Token, "clean", "receipt", "") }()
	}
	wg.Wait()
	close(errs)
	wins := 0
	for e := range errs {
		if e == nil {
			wins++
		} else if !errors.Is(e, ErrLeaseInvalid) {
			t.Fatal(e)
		}
	}
	if wins != 1 {
		t.Fatal("refunds", wins)
	}
	l, _ = s.ClaimTask("ra", "t", "w", "thread", time.Minute)
	now := s.clock()
	s.now = func() time.Time { return now.Add(2 * time.Minute) }
	if err := s.YieldTaskLease("ra", "t", l.Token, "clean", "receipt", ""); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatal("expired refund", err)
	}
	e, _ := s.TaskEligibility("ra", "t")
	if e.Attempts != 1 {
		t.Fatal(e)
	}
}
func TestTaskYieldFourthPhaseNeedsNoReset(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "ra", TaskID: "phase", Subject: "phased delivery"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		l, err := s.ClaimTask("ra", "phase", "w", "thread", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.YieldTaskLease("ra", "phase", l.Token, "clean", "receipt:phase", ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.ClaimTask("ra", "phase", "w", "thread", time.Minute); err != nil {
		t.Fatal("fourth clean phase refused", err)
	}
}
func TestTaskYieldNoReasonHeuristics(t *testing.T) {
	s := newTestStore(t)
	_ = s.AddTask(Task{Agent: "ra", TaskID: "t", Subject: "work"})
	for i := 0; i < 3; i++ {
		l, err := s.ClaimTask("ra", "t", "w", "thread", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err = s.ReleaseTaskLease("ra", "t", l.Token, "clean external wait; success; please refund"); err != nil {
			t.Fatal(err)
		}
	}
	e, _ := s.TaskEligibility("ra", "t")
	if e.Attempts != 3 || e.Status != "blocked" {
		t.Fatal("reason-based refund", e)
	}
}

func TestTaskYieldWireOwner(t *testing.T) {
	var logs strings.Builder
	backend, client := ruleHarness(t, "enforce", &logs)
	host, _ := os.Hostname()
	now := time.Date(2026, 9, 10, 15, 0, 0, 0, time.UTC)
	register(t, backend, "own", "ra", host, "active", now)
	own := client("ra", "own")
	if err := backend.AddTask(Task{Agent: "ra", TaskID: "wire", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	l, err := own.ClaimTask("ra", "wire", "worker", "own", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	register(t, backend, "other", "ra", host, "active", now)
	other := client("ra", "other")
	if err = other.YieldTaskLease("ra", "wire", l.Token, "clean", "receipt", ""); err == nil {
		t.Fatal("foreign session refunded")
	}
	own = client("ra", "own")
	if err = own.YieldTaskLease("ra", "wire", l.Token, "clean", "receipt", ""); err != nil {
		t.Fatal(err)
	}
	e, err := backend.TaskEligibility("ra", "wire")
	if err != nil || e.Attempts != 0 || e.LeaseHeld {
		t.Fatal(e, err)
	}
}

func TestTaskYieldAuditFailureRollsBack(t *testing.T) {
	s := newTestStore(t)
	if err := s.AddTask(Task{Agent: "ra", TaskID: "audit", Subject: "work"}); err != nil {
		t.Fatal(err)
	}
	l, err := s.ClaimTask("ra", "audit", "w", "thread", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// Isolated test ledger only: an unavailable audit sink must not refund work.
	if _, err = s.exec(`DROP TABLE task_lease_log`); err != nil {
		t.Fatal(err)
	}
	if err = s.YieldTaskLease("ra", "audit", l.Token, "clean", "receipt", ""); err == nil {
		t.Fatal("yield without audit")
	}
	e, err := s.TaskEligibility("ra", "audit")
	if err != nil || !e.LeaseHeld || e.Attempts != 1 {
		t.Fatal("refund escaped rollback", e, err)
	}
}
