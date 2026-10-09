package namespec

import "testing"

func gen(n int) *int { return &n }

func TestResolveDelivery_CanonicalDeliversWhenGenerationMatchesOrUnresolved(t *testing.T) {
	record := NameRecord{State: StateCanonical, ThreadID: "t1", Generation: 1}

	d, err := ResolveDelivery(record, nil)
	if err != nil || d.Status != DeliveryOK || d.ThreadID != "t1" {
		t.Fatalf("legacy send to canonical should deliver, got %+v err=%v", d, err)
	}
	d, err = ResolveDelivery(record, gen(1))
	if err != nil || d.Status != DeliveryOK {
		t.Fatalf("matching-generation send to canonical should deliver, got %+v err=%v", d, err)
	}
}

func TestResolveDelivery_ActiveAliasAlwaysRedirectsToCanonical(t *testing.T) {
	// Rename A->B: A becomes an active alias of the still-live thread.
	record := NameRecord{State: StateActiveAlias, ThreadID: "t1", Generation: 1, CanonicalName: "claude-router-b"}
	d, err := ResolveDelivery(record, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryRenamed || d.To != "claude-router-b" || d.ThreadID != "t1" || d.Generation != 1 {
		t.Fatalf("expected a renamed redirect carrying the target's thread-id+generation, got %+v", d)
	}
}

func TestAuthorizeReuse_RefusedWhileActiveAlias(t *testing.T) {
	// Attempted reuse of A while A is still an active alias of the live
	// thread must be refused -- collision policy, not a race to win.
	if err := AuthorizeReuse(StateActiveAlias); err == nil {
		t.Fatal("reuse of a name that is still an active alias must be refused")
	}
}

func TestResolveDelivery_ExpiredAliasRefusesDeliveryNeverRedirects(t *testing.T) {
	// Alias expiry while the thread stays live: A becomes an expired alias,
	// not a tombstone; delivery to A is refused with the live thread's
	// current canonical name, never redirected.
	record := NameRecord{State: StateExpiredAlias, ThreadID: "t1", Generation: 1, CanonicalName: "claude-router-b"}
	d, err := ResolveDelivery(record, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryAliasExpired {
		t.Fatalf("expected alias-expired, got %+v", d)
	}
	if d.Status == DeliveryRenamed {
		t.Fatal("an expired alias must never be redirected like an active one")
	}
	if d.Canonical != "claude-router-b" || d.ThreadID != "t1" || d.Generation != 1 {
		t.Fatalf("alias-expired must name the live thread's current canonical/thread_id/generation, got %+v", d)
	}
}

func TestAuthorizeReuse_RefusedWhileExpiredAlias(t *testing.T) {
	if err := AuthorizeReuse(StateExpiredAlias); err == nil {
		t.Fatal("reuse of a name that is still an expired alias (thread live elsewhere) must be refused")
	}
}

func TestResolveDelivery_RetiredTombstoneNotYetReused(t *testing.T) {
	record := NameRecord{State: StateTombstone, Successor: "claude-router-c"}
	d, err := ResolveDelivery(record, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryRetired || d.Successor != "claude-router-c" {
		t.Fatalf("expected retired with successor, got %+v", d)
	}
}

func TestAuthorizeReuse_AllowedOnlyFromTombstone(t *testing.T) {
	if err := AuthorizeReuse(StateTombstone); err != nil {
		t.Fatalf("reuse from a tombstone should be allowed, got: %v", err)
	}
	if err := AuthorizeReuse(StateCanonical); err == nil {
		t.Fatal("reuse while canonical must be refused")
	}
}

func TestResolveDelivery_ReusedTombstoneLegacySendIsAmbiguous(t *testing.T) {
	// A name-only send (no resolved generation) against a reused name must
	// be rejected as ambiguous -- the router never guesses which generation
	// a bare name means once more than one has existed.
	record := NameRecord{State: StateTombstone, Reused: true, ThreadID: "t2-new-holder", Generation: 2}
	d, err := ResolveDelivery(record, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryAmbiguous {
		t.Fatalf("expected ambiguous for an unresolved legacy send, got %+v", d)
	}
}

func TestResolveDelivery_ReusedTombstoneStaleGenerationRejectedNeverCrossDelivered(t *testing.T) {
	// A sender who resolved an expected generation before the reuse must be
	// rejected as stale, never silently delivered into the new holder's
	// inbox.
	record := NameRecord{State: StateTombstone, Reused: true, ThreadID: "t2-new-holder", Generation: 2}
	d, err := ResolveDelivery(record, gen(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryStale || d.ExpectedGeneration != 1 || d.CurrentGeneration != 2 {
		t.Fatalf("expected a stale rejection naming both generations, got %+v", d)
	}
}

func TestResolveDelivery_ReusedTombstoneMatchingGenerationDeliversToCurrentHolder(t *testing.T) {
	record := NameRecord{State: StateTombstone, Reused: true, ThreadID: "t2-new-holder", Generation: 2}
	d, err := ResolveDelivery(record, gen(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryOK || d.ThreadID != "t2-new-holder" {
		t.Fatalf("a correctly-resolved current-generation send must deliver to the current holder, got %+v", d)
	}
}

func TestResolveDelivery_CanonicalStaleGenerationRejected(t *testing.T) {
	record := NameRecord{State: StateCanonical, ThreadID: "t1", Generation: 3}
	d, err := ResolveDelivery(record, gen(2))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Status != DeliveryStale || d.ExpectedGeneration != 2 || d.CurrentGeneration != 3 {
		t.Fatalf("expected a stale rejection, got %+v", d)
	}
}
