package namespec

import (
	"strings"
	"testing"
	"time"
)

// This file composes C2 (schema trust), C3 (machine proof), C4 (rename
// idempotency+CAS), and C5 (alias/tombstone delivery) together against the
// adversarial scenarios ADR-072 C6 names, at the pure-decision-layer
// boundary this package owns. It demonstrates that the composed guards
// refuse each scenario end-to-end at this layer -- it is NOT the full C6
// requirement, which the ADR scopes to "run against the actual store/
// service boundary with production constraints enabled" (serve.go wiring,
// not yet built). A35: this file's claim is bounded to what it actually
// exercises -- the pure composition -- not the live store/service.

func TestAdversarial_CrossHostSpoofingRefusedEvenWithValidSchemaAndReceipt(t *testing.T) {
	// An m5 session tries to register an m1 name. The schema/receipt side
	// (C2) is entirely valid; only C3's machine-proof check must catch this.
	active := PromotionReceipt{SchemaVersion: 1, SchemaHash: "hash-v1", PromotedBy: "owner", PromotedAt: time.Now()}
	if err := active.ValidateAgainstActive(1, "hash-v1"); err != nil {
		t.Fatalf("schema check should pass (not the thing under test): %v", err)
	}
	name, err := Construct("claude", "router", "m1", "")
	if err != nil {
		t.Fatalf("unexpected construct error: %v", err)
	}
	err = AuthorizeMachineClaim(name.Machine, RegisteringSessionCredential{MachineID: "uuid-m5"}, "uuid-m1")
	if err == nil || !strings.Contains(err.Error(), "cross-host claim refused") {
		t.Fatalf("expected the m5 session to be refused an m1 name despite valid schema, got: %v", err)
	}
}

func TestAdversarial_StaleWorkingTreeRegistryDivergenceRefused(t *testing.T) {
	active := PromotionReceipt{SchemaVersion: 2, SchemaHash: "hash-v2", PromotedBy: "owner", PromotedAt: time.Now()}
	// A working tree still on v2 but with a locally-edited (divergent) hash.
	if err := active.ValidateAgainstActive(2, "hash-v2-divergent"); err == nil || !strings.Contains(err.Error(), "working-tree divergence") {
		t.Fatalf("expected working-tree divergence refusal, got: %v", err)
	}
}

func TestAdversarial_UnsupportedDowngradedSchemaAndRevokedCredentialBothRefused(t *testing.T) {
	active := PromotionReceipt{SchemaVersion: 3, SchemaHash: "hash-v3", PromotedBy: "owner", PromotedAt: time.Now()}
	if err := active.ValidateAgainstActive(2, "hash-v2"); err == nil || !strings.Contains(err.Error(), "unauthorized downgrade") {
		t.Fatalf("expected unauthorized-downgrade refusal, got: %v", err)
	}
	if err := active.ValidateAgainstActive(4, "hash-v4"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Fatalf("expected unsupported-version refusal, got: %v", err)
	}
	// Even a session presenting the CURRENT active version/hash is refused
	// if its machine-id credential has since been revoked (ADR-067).
	if err := active.ValidateAgainstActive(3, "hash-v3"); err != nil {
		t.Fatalf("schema check should pass for the revoked-credential case: %v", err)
	}
	err := AuthorizeMachineClaim("m1", RegisteringSessionCredential{MachineID: "uuid-m1", Revoked: true}, "uuid-m1")
	if err == nil || !strings.Contains(err.Error(), "revoked") {
		t.Fatalf("expected revoked-credential refusal even with a matching machine-id, got: %v", err)
	}
}

func TestAdversarial_DuplicateLiveClaimRaceOneWinsOneConflicts(t *testing.T) {
	// Two sessions race to claim the same thread's rename (one name, one
	// thread). Both read revision 1; the store commits one first.
	sessionA := RenameAttempt{IdempotencyKey: "kA", RequestDigest: "dA", ExpectedRevision: 1}
	sessionB := RenameAttempt{IdempotencyKey: "kB", RequestDigest: "dB", ExpectedRevision: 1}

	outcome, _, err := DecideRename(nil, sessionA, 1)
	if err != nil || outcome != RenameProceed {
		t.Fatalf("session A should win the race, got outcome=%v err=%v", outcome, err)
	}
	// Store is now at revision 2 after A's commit.
	_, _, err = DecideRename(nil, sessionB, 2)
	if err == nil || !strings.Contains(err.Error(), "stale revision conflict") {
		t.Fatalf("session B must get a stale-revision conflict, not a silent overwrite, got: %v", err)
	}
}

func TestAdversarial_SameKeySamePayloadRetryIsNoOpReplay(t *testing.T) {
	prior := &IdempotencyRecord{Digest: "d1", Receipt: "receipt-1"}
	outcome, receipt, err := DecideRename(prior, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 1}, 2)
	if err != nil || outcome != RenameReplay || receipt != "receipt-1" {
		t.Fatalf("same key+payload retry must replay unchanged, got outcome=%v receipt=%q err=%v", outcome, receipt, err)
	}
}

func TestAdversarial_SameKeyDifferentPayloadRetryRejected(t *testing.T) {
	prior := &IdempotencyRecord{Digest: "d1", Receipt: "receipt-1"}
	_, _, err := DecideRename(prior, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d2", ExpectedRevision: 1}, 1)
	if err == nil || !strings.Contains(err.Error(), "already used for a different request") {
		t.Fatalf("same key, different payload must be rejected, got: %v", err)
	}
}

func TestAdversarial_CrashMidwayResumeReturnsSameReceiptBeforeAndAfterCommit(t *testing.T) {
	// Before commit: no record yet exists under the key -- a retry proceeds
	// (this models a crash before the transaction committed; the real retry
	// is a fresh attempt, not a replay, because nothing was recorded).
	outcomeBefore, _, err := DecideRename(nil, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 5}, 5)
	if err != nil || outcomeBefore != RenameProceed {
		t.Fatalf("pre-crash (no record) retry should proceed, got outcome=%v err=%v", outcomeBefore, err)
	}
	// After commit: the record now exists; a retry (crash after commit, or a
	// slow reply) must return the SAME receipt, not re-apply or conflict.
	committed := &IdempotencyRecord{Digest: "d1", Receipt: "receipt-committed"}
	outcomeAfter, receiptAfter, err := DecideRename(committed, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 5}, 6)
	if err != nil || outcomeAfter != RenameReplay || receiptAfter != "receipt-committed" {
		t.Fatalf("post-crash-after-commit retry must replay the same receipt, got outcome=%v receipt=%q err=%v", outcomeAfter, receiptAfter, err)
	}
}

func TestAdversarial_DuplicateCanonicalAliasTombstoneCollisionRefused(t *testing.T) {
	// A name currently an active alias of a live thread cannot be reused as
	// if it were a free tombstone slot -- the single collision policy spans
	// all three non-tombstone states.
	for _, st := range []NameState{StateCanonical, StateActiveAlias, StateExpiredAlias} {
		if err := AuthorizeReuse(st); err == nil {
			t.Fatalf("reuse must be refused while state is %s", st)
		}
	}
	if err := AuthorizeReuse(StateTombstone); err != nil {
		t.Fatalf("reuse must be allowed from a tombstone, got: %v", err)
	}
}

func TestAdversarial_RetiredNameReuseGetsNewGenerationTombstoneSuperseded(t *testing.T) {
	// The tombstone record is superseded (Reused=true, new Generation,
	// new ThreadID) rather than deleted -- the old generation's identity
	// stays answerable via a stale-generation send (next test), not erased.
	reused := NameRecord{State: StateTombstone, Reused: true, ThreadID: "new-holder-thread", Generation: 2}
	d, err := ResolveDelivery(reused, gen(2))
	if err != nil || d.Status != DeliveryOK || d.ThreadID != "new-holder-thread" {
		t.Fatalf("current-generation send after reuse must deliver to the new holder, got %+v err=%v", d, err)
	}
}

func TestAdversarial_StaleGenerationAndAmbiguousLegacySendBothRejectedNeverGuessed(t *testing.T) {
	reused := NameRecord{State: StateTombstone, Reused: true, ThreadID: "new-holder-thread", Generation: 2}

	staleSend, err := ResolveDelivery(reused, gen(1))
	if err != nil || staleSend.Status != DeliveryStale {
		t.Fatalf("a sender holding the old generation must be rejected as stale, not cross-delivered, got %+v err=%v", staleSend, err)
	}
	legacySend, err := ResolveDelivery(reused, nil)
	if err != nil || legacySend.Status != DeliveryAmbiguous {
		t.Fatalf("a legacy bare-name send against a reused name must be ambiguous, never guessed, got %+v err=%v", legacySend, err)
	}
}

func TestAdversarial_TaskComponentBindingRejectionIsOutOfNamespecScope(t *testing.T) {
	// P1's grammar check validates the task slot's SHAPE only and grants no
	// binding authority (C6 phase-order note) -- a syntactically valid task
	// string still parses even though binding it to a real, authorized,
	// durable task/workstream id is the router's job, not namespec's. This
	// test documents the boundary rather than a refusal namespec itself
	// performs.
	n, err := Parse("claude-router-m1-some-unauthorized-task-id")
	if err != nil {
		t.Fatalf("a shape-valid task slot must still parse, got: %v", err)
	}
	if n.Task != "some-unauthorized-task-id" {
		t.Fatalf("expected the task slot to be glean-parsed, got %q", n.Task)
	}
	// namespec has no authority API over task bindings; the router's own
	// task registry is where "authorized, existing, durable" is enforced.
}
