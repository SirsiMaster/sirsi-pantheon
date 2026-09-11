package routerstore

import (
	"errors"
	"testing"
	"time"
)

// TestBreakerAutoHalfOpenAfterCooldown proves the fix for "the router rejected
// delivery before append because its existing circuit breaker is open": a
// tripped breaker rejects only while inside BreakerCooldown, then self-heals.
// Without the fix the breaker latched open until an operator ran breaker-reset,
// so a transient fault permanently paused dispatch for that domain.
func TestBreakerAutoHalfOpenAfterCooldown(t *testing.T) {
	s := openTestStore(t)
	now := time.Date(2026, 9, 11, 3, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	oldT := BreakerThreshold
	BreakerThreshold = 2
	t.Cleanup(func() { BreakerThreshold = oldT })

	trip := func() {
		tx, err := s.beginImmediate()
		if err != nil {
			t.Fatal(err)
		}
		if err := s.recordFailureTx(tx, s.clock(), "sender:x"); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
	trip()
	trip() // failures >= threshold → tripped

	gate := func() error {
		tx, err := s.beginImmediate()
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback() }()
		if err := s.breakerGateTx(tx, s.clock(), "sender:x"); err != nil {
			return err
		}
		return tx.Commit()
	}

	// Inside the cooldown the breaker still gates.
	if err := gate(); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("tripped breaker must reject within cooldown, got %v", err)
	}

	// Once the cooldown elapses the gate half-opens, admits the probe, and
	// clears the trip so a recovered domain stays closed.
	now = now.Add(BreakerCooldown + time.Second)
	if err := gate(); err != nil {
		t.Fatalf("after cooldown the gate must half-open, got %v", err)
	}
	bs, err := s.Breakers()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bs {
		if b.Domain == "sender:x" && b.TrippedAt != "" {
			t.Fatalf("half-open must clear the trip, still tripped_at=%q", b.TrippedAt)
		}
	}
}
