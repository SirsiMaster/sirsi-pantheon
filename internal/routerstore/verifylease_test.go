package routerstore

// VerifyLease negative controls (router item 20261001-031502, Ra ACCEPTED
// 2026-10-01): a read-only, token-fenced check so a caller (the MCP server's
// router_close path) can confirm its claim is still live before acting on it,
// instead of trusting a process-local map that never expires. Each control
// below is a way an unfenced "trust the local map" check would wrongly let a
// stale or superseded claim through CloseItem.

import (
	"testing"
	"time"
)

// Positive control: a freshly claimed, unexpired lease verifies true.
func TestVerifyLeaseValidLeaseIsTrue(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "x"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.VerifyLease(lease.ItemID, lease.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("a live, correctly-tokened lease must verify true")
	}
}

// Negative control 1: expired lease verifies false, not true.
func TestVerifyLeaseExpiredLeaseIsFalse(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "x"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	expired := s.now().Add(2 * time.Hour)
	s.now = func() time.Time { return expired }
	ok, err := s.VerifyLease(lease.ItemID, lease.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("an expired lease must verify false")
	}
}

// Negative control 2: a mismatched token (never issued) verifies false.
func TestVerifyLeaseWrongTokenIsFalse(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "x"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := s.VerifyLease(lease.ItemID, "not-the-real-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("a mismatched token must verify false")
	}
}

// Negative control 3: a reassigned lease — the ORIGINAL token no longer
// verifies once the item has been reclaimed under a new token. This is the
// exact race the proposal exists to close: instance A's stale local map must
// not let it believe its claim is still current.
func TestVerifyLeaseReassignedLeaseOldTokenIsFalse(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "x"); err != nil {
		t.Fatal(err)
	}
	leaseA, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// A's lease expires; ClaimNext's own expired-lease reclaim returns the
	// item to open, and B claims it fresh.
	expired := s.now().Add(2 * time.Hour)
	s.now = func() time.Time { return expired }
	leaseB, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if leaseB.Token == leaseA.Token {
		t.Fatal("test setup: B must receive a new token, not A's stale one")
	}

	okA, err := s.VerifyLease(leaseA.ItemID, leaseA.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if okA {
		t.Fatal("A's superseded token must not verify after B's reclaim")
	}
	okB, err := s.VerifyLease(leaseB.ItemID, leaseB.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !okB {
		t.Fatal("B's current token must verify true")
	}
}

// Negative control 4: a terminal item (already completed) verifies false even
// against the token that legitimately completed it — terminal is terminal.
func TestVerifyLeaseTerminalItemIsFalse(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "x"); err != nil {
		t.Fatal(err)
	}
	lease, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if completeErr := s.Complete(lease.ItemID, lease.Token, "done"); completeErr != nil {
		t.Fatal(completeErr)
	}
	ok, err := s.VerifyLease(lease.ItemID, lease.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("a terminal item must verify false even for the completing token")
	}
}

// Unknown item id: no item ever existed at this id. VerifyLease reports a
// clean false, not an error — the caller (router_close) wants one refusal
// path for "not safe to close", not a second error-handling branch for this
// case versus the fencing cases above.
func TestVerifyLeaseUnknownItemIsFalse(t *testing.T) {
	s := newTestStore(t)
	ok, err := s.VerifyLease("never-existed", "whatever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("an unknown item id must verify false")
	}
}

// Two-instance race test (Ra's required evidence, item 20261001-031502): A's
// lease expires, B reclaims, A's close must refuse (via VerifyLease) and B's
// result must survive — proving the fence actually prevents the clobber the
// proposal names, not just that VerifyLease returns the right bool in
// isolation.
func TestVerifyLeaseRacePreventsStaleCloseFromClobberingReclaim(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Send("owner", "codex-home", "item", "review", "x"); err != nil {
		t.Fatal(err)
	}
	leaseA, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// A's lease expires; B (a second instance of the same agent) reclaims it.
	expired := s.now().Add(2 * time.Hour)
	s.now = func() time.Time { return expired }
	leaseB, err := s.ClaimNext("codex-home", time.Minute)
	if err != nil {
		t.Fatal(err)
	}

	// A, unaware its lease expired (its local map never evicted), checks
	// before closing — the fenced check must refuse.
	okA, err := s.VerifyLease(leaseA.ItemID, leaseA.Token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if okA {
		t.Fatal("A must be refused verification before it can clobber B's claim")
	}
	// A correctly refuses to call CloseItem. B proceeds and completes.
	if completeErr := s.Complete(leaseB.ItemID, leaseB.Token, "B's result"); completeErr != nil {
		t.Fatalf("B's completion under its own current lease must succeed: %v", completeErr)
	}

	item, err := s.Get(leaseB.ItemID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Status != StatusCompleted {
		t.Fatalf("status = %q, want completed", item.Status)
	}
	if item.Result != "B's result" {
		t.Fatalf("result = %q, want B's result to survive uncontested", item.Result)
	}
}
