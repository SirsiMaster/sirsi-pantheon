package routerstore

import (
	"testing"
	"time"
)

// Ra charter M0 T4 (A35): the wake-latency sample must be first claim minus
// opened, not last-lease-update minus opened. These tests are the negative
// control for the old lease_updated-based query: StartWork and RenewLease
// rewrite lease_updated, so a report that reads it inflates every item that
// was started or renewed.

func TestWakeLatencyIgnoresStartWorkAndRenew(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if _, err := s.Send("sender", "ra", "T", "proposal", "body"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(37 * time.Second)
	lease, err := s.ClaimNext("ra", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(60 * time.Second)
	if err := s.StartWork(lease.ItemID, lease.Token); err != nil {
		t.Fatal(err)
	}
	now = now.Add(60 * time.Second)
	if err := s.RenewLease(lease.ItemID, lease.Token, 10*time.Minute); err != nil {
		t.Fatal(err)
	}
	samples, err := s.WakeLatencySamples("ra", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].LatencySeconds != 37 {
		t.Fatalf("want one sample of 37s (first claim), got %+v", samples)
	}
}

func TestWakeLatencyFirstClaimSurvivesRequeueAndReclaim(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if _, err := s.Send("sender", "ra", "T", "proposal", "body"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Second)
	lease, err := s.ClaimNext("ra", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Fail(lease.ItemID, lease.Token, "retry", "transient"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(100 * time.Second)
	if _, err := s.ClaimNext("ra", 10*time.Minute); err != nil {
		t.Fatalf("re-claim after requeue: %v", err)
	}
	samples, err := s.WakeLatencySamples("ra", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].LatencySeconds != 10 {
		t.Fatalf("want one sample of 10s (first claim preserved), got %+v", samples)
	}
}

func TestWakeLatencyExcludesEmptyFirstClaim(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if _, err := s.Send("sender", "ra", "T", "proposal", "body"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(5 * time.Second)
	if _, err := s.ClaimNext("ra", time.Minute); err != nil {
		t.Fatal(err)
	}
	// Simulate a row claimed before v27: claimed, lease_updated set, no first_claimed_at.
	if _, err := s.db.Exec(`UPDATE items SET first_claimed_at='' WHERE to_agent=?`, "ra"); err != nil {
		t.Fatal(err)
	}
	samples, err := s.WakeLatencySamples("ra", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatalf("pre-v27 row (first_claimed_at='') must be excluded, got %+v", samples)
	}
}

func TestWakeLatencyClaimItemSetsFirstClaim(t *testing.T) {
	s := newTestStore(t)
	now := time.Date(2026, 8, 6, 2, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	id, err := s.Send("sender", "ra", "T", "proposal", "body")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(12 * time.Second)
	if _, err := s.ClaimItem("ra", id, time.Minute); err != nil {
		t.Fatal(err)
	}
	samples, err := s.WakeLatencySamples("ra", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || samples[0].LatencySeconds != 12 || samples[0].FirstClaimedAt == "" {
		t.Fatalf("ClaimItem must set first_claimed_at; got %+v", samples)
	}
}
