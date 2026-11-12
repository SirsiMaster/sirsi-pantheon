package routerstore

import (
	"testing"
	"time"
)

// allStatuses is the complete status vocabulary of lease.go. Adding a status
// constant there without adding it here AND to DeliveryStateOf must fail.
var allStatuses = []string{
	StatusOpen, StatusClaimed, StatusWorking, StatusBlocked,
	StatusDeadLetter, StatusCompleted, StatusClosed,
}

func TestDeliveryStateOfTable(t *testing.T) {
	const ts = "2026-11-11T00:00:00Z"
	cases := []struct {
		status, acked string
		want          DeliveryState
	}{
		{StatusOpen, "", DeliverySubmitted},
		{StatusOpen, ts, DeliveryAcknowledged},
		{StatusClaimed, "", DeliveryWorking},
		{StatusClaimed, ts, DeliveryWorking},
		{StatusWorking, "", DeliveryWorking},
		{StatusWorking, ts, DeliveryWorking},
		{StatusBlocked, "", DeliveryBlocked},
		{StatusBlocked, ts, DeliveryBlocked},
		{StatusCompleted, "", DeliveryCompleted},
		{StatusCompleted, ts, DeliveryCompleted},
		{StatusDeadLetter, "", DeliveryFailed},
		{StatusDeadLetter, ts, DeliveryFailed},
		{StatusClosed, "", DeliveryClosed},
		{StatusClosed, ts, DeliveryClosed},
		{"bogus", "", DeliveryUnknown},
		{"", ts, DeliveryUnknown},
	}
	for _, c := range cases {
		if got := DeliveryStateOf(Item{Status: c.status, AckedAt: c.acked}); got != c.want {
			t.Errorf("status=%q acked=%q: got %s want %s", c.status, c.acked, got, c.want)
		}
	}
}

// Every status constant in lease.go must map to a known state.
func TestDeliveryStateCoversEveryStatus(t *testing.T) {
	for _, st := range allStatuses {
		if DeliveryStateOf(Item{Status: st}) == DeliveryUnknown {
			t.Errorf("status %q has no delivery-state mapping", st)
		}
	}
}

func TestDeliveryStateFollowsLifecycle(t *testing.T) {
	s := openTestStore(t)
	check := func(id string, want DeliveryState) {
		t.Helper()
		it, err := s.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := DeliveryStateOf(it); got != want {
			t.Fatalf("item %s: got %s want %s (status=%q acked=%q)", id, got, want, it.Status, it.AckedAt)
		}
	}

	id, err := s.Send("seed", "worker", "t", "", "do it")
	if err != nil {
		t.Fatal(err)
	}
	check(id, DeliverySubmitted)
	if err := s.AckItem(id); err != nil {
		t.Fatal(err)
	}
	check(id, DeliveryAcknowledged)
	l, err := s.ClaimNext("worker", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	check(id, DeliveryWorking)
	if err := s.StartWork(id, l.Token); err != nil {
		t.Fatal(err)
	}
	check(id, DeliveryWorking)
	if err := s.Complete(id, l.Token, "done"); err != nil {
		t.Fatal(err)
	}
	check(id, DeliveryCompleted)

	// Blocked path.
	bid, err := s.Send("seed", "worker2", "t", "", "x")
	if err != nil {
		t.Fatal(err)
	}
	bl, err := s.ClaimNext("worker2", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Block(bid, bl.Token, "waiting"); err != nil {
		t.Fatal(err)
	}
	check(bid, DeliveryBlocked)

	// Dead-letter path: fail until the retry ceiling.
	fid, err := s.Send("seed", "worker3", "t", "", "y")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxRetriesPerItem; i++ {
		fl, err := s.ClaimNext("worker3", time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Fail(fid, fl.Token, "rc=1", "build_error"); err != nil {
			t.Fatal(err)
		}
		if i < MaxRetriesPerItem-1 {
			check(fid, DeliverySubmitted) // back to open, never acked
		}
	}
	check(fid, DeliveryFailed)
}
