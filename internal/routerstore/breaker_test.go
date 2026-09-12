package routerstore

import (
	"errors"
	"testing"
	"time"
)

// TestBreakerTimedResetAfterCooldown proves the fix for "the router rejected
// delivery before append because its existing circuit breaker is open": a
// tripped breaker rejects only while inside BreakerCooldown, then does a TIMED
// FULL RESET. Without the fix the breaker latched open until an operator ran
// breaker-reset, so a transient fault permanently paused dispatch for that
// domain.
//
// This is NOT a single-probe half-open and the test asserts the real semantics:
// after the cooldown the trip AND failure count are cleared, so MULTIPLE calls
// pass (not just one probe). It also asserts the trip left a RETRIEVABLE cause
// receipt in the items table (Stack Lab convention: every trip is inspectable).
func TestBreakerTimedResetAfterCooldown(t *testing.T) {
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

	// The trip must leave a retrievable cause receipt: the breaker row's
	// operator_item must name an item that actually exists in the items table.
	brs, err := s.Breakers()
	if err != nil {
		t.Fatal(err)
	}
	var opItem string
	for _, b := range brs {
		if b.Domain == "sender:x" {
			opItem = b.OperatorItem
		}
	}
	if opItem == "" {
		t.Fatal("tripped breaker must record an operator_item cause receipt")
	}
	if _, gerr := s.Get(opItem); gerr != nil {
		t.Fatalf("breaker cause receipt %q must resolve to a real item, got %v", opItem, gerr)
	}

	// Inside the cooldown the breaker still gates.
	if gerr := gate(); !errors.Is(gerr, ErrBreakerOpen) {
		t.Fatalf("tripped breaker must reject within cooldown, got %v", gerr)
	}

	// Once the cooldown elapses the gate does a timed full reset: it clears the
	// trip and failure count, so this call AND a subsequent one both pass (this
	// is a full reset, not a single probe).
	now = now.Add(BreakerCooldown + time.Second)
	if gerr := gate(); gerr != nil {
		t.Fatalf("after cooldown the first call must pass (timed reset), got %v", gerr)
	}
	if gerr := gate(); gerr != nil {
		t.Fatalf("after a timed reset a second call must also pass (not single-probe), got %v", gerr)
	}
	bs, err := s.Breakers()
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bs {
		if b.Domain == "sender:x" && b.TrippedAt != "" {
			t.Fatalf("timed reset must clear the trip, still tripped_at=%q", b.TrippedAt)
		}
	}
}

// TestBreakerRecoversThroughPublicClaimPath is SSA's requested public-path
// coverage: drive a target breaker to trip through the real ClaimNext/Fail
// dead-letter path (not a direct gate reset), confirm ClaimNext rejects while
// tripped, then — after the cooldown — confirm ClaimNext admits work again.
// This exercises the sustained-failure → recovery cycle end to end.
func TestBreakerRecoversThroughPublicClaimPath(t *testing.T) {
	s := openTestStore(t)
	now := time.Date(2026, 9, 11, 4, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	oldT := BreakerThreshold
	BreakerThreshold = 2
	t.Cleanup(func() { BreakerThreshold = oldT })
	oldR := MaxRetriesPerItem
	MaxRetriesPerItem = 1 // each Fail dead-letters immediately → records a breaker failure
	t.Cleanup(func() { MaxRetriesPerItem = oldR })

	// Seed enough work that dead-lettering a few items trips target:worker.
	seedOpen(t, s, "worker", 5)

	// Claim then Fail, driving dead-letters until the target breaker trips.
	for i := 0; i < BreakerThreshold+1; i++ {
		lease, err := s.ClaimNext("worker", time.Minute)
		if errors.Is(err, ErrBreakerOpen) {
			break // already tripped
		}
		if err != nil {
			t.Fatalf("claim %d: %v", i, err)
		}
		if err := s.Fail(lease.ItemID, lease.Token, "delivery fault", "downstream"); err != nil {
			t.Fatalf("fail %d: %v", i, err)
		}
	}

	// Tripped: ClaimNext through the target must now reject.
	if _, err := s.ClaimNext("worker", time.Minute); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("tripped target breaker must pause claims, got %v", err)
	}

	// After the cooldown the timed reset lets the public claim path work again.
	now = now.Add(BreakerCooldown + time.Second)
	if _, err := s.ClaimNext("worker", time.Minute); err != nil && !errors.Is(err, ErrNotFound) {
		// ErrNotFound is acceptable only if no claimable work remains; a breaker
		// rejection is not.
		if errors.Is(err, ErrBreakerOpen) {
			t.Fatalf("after cooldown the target breaker must have reset, still ErrBreakerOpen")
		}
		t.Fatalf("after cooldown ClaimNext: unexpected error %v", err)
	}
}
