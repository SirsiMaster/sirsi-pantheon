package namespec

import (
	"strings"
	"testing"
)

func TestRequestDigest_StableAcrossKeyOrder(t *testing.T) {
	a, err := RequestDigest([]byte(`{"thread_id":"t1","old_name":"claude-router-m1","new_name":"claude-router-m1-r2"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := RequestDigest([]byte(`{"new_name":"claude-router-m1-r2","thread_id":"t1","old_name":"claude-router-m1"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != b {
		t.Fatalf("digest must be key-order independent, got %q vs %q", a, b)
	}
}

func TestRequestDigest_DetectsRealChange(t *testing.T) {
	a, _ := RequestDigest([]byte(`{"thread_id":"t1","new_name":"claude-router-m1-r2"}`))
	b, _ := RequestDigest([]byte(`{"thread_id":"t1","new_name":"claude-router-m1-r3"}`))
	if a == b {
		t.Fatal("digest must differ for a different requested change")
	}
}

func TestDecideRename_ProceedsWhenNoPriorAndRevisionMatches(t *testing.T) {
	outcome, receipt, err := DecideRename(nil, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 5}, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != RenameProceed || receipt != "" {
		t.Fatalf("expected RenameProceed with no receipt, got outcome=%v receipt=%q", outcome, receipt)
	}
}

func TestDecideRename_StaleRevisionConflictWhenNoPriorAndRevisionMismatches(t *testing.T) {
	_, _, err := DecideRename(nil, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 5}, 6)
	if err == nil || !strings.Contains(err.Error(), "stale revision conflict") {
		t.Fatalf("expected a stale-revision conflict, got: %v", err)
	}
}

func TestDecideRename_ReplayReturnsOriginalReceiptSameKeySameDigest(t *testing.T) {
	prior := &IdempotencyRecord{Digest: "d1", Receipt: "receipt-xyz"}
	outcome, receipt, err := DecideRename(prior, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 5}, 999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != RenameReplay || receipt != "receipt-xyz" {
		t.Fatalf("expected RenameReplay with the original receipt, got outcome=%v receipt=%q", outcome, receipt)
	}
}

func TestDecideRename_IdempotencyCheckedBeforeCAS_ReplayEvenWhenExpectedRevisionIsStale(t *testing.T) {
	// The caller's own prior commit moved the revision past what it expected
	// when it first issued the request; a retry of that SAME request must
	// still be recognized as a replay, not rejected as a stale-revision
	// conflict for colliding with the very commit it caused.
	prior := &IdempotencyRecord{Digest: "d1", Receipt: "receipt-xyz"}
	outcome, receipt, err := DecideRename(prior, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d1", ExpectedRevision: 5}, 6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != RenameReplay || receipt != "receipt-xyz" {
		t.Fatalf("a same-key-same-digest retry must replay regardless of CAS staleness, got outcome=%v receipt=%q err=%v", outcome, receipt, err)
	}
}

func TestDecideRename_KeyReuseWithDifferentDigestIsRejected(t *testing.T) {
	prior := &IdempotencyRecord{Digest: "d1", Receipt: "receipt-xyz"}
	outcome, receipt, err := DecideRename(prior, RenameAttempt{IdempotencyKey: "k1", RequestDigest: "d2", ExpectedRevision: 5}, 5)
	if err == nil || !strings.Contains(err.Error(), "already used for a different request") {
		t.Fatalf("a key reused for a different payload must be rejected, got outcome=%v receipt=%q err=%v", outcome, receipt, err)
	}
}

func TestDecideRename_TwoRacingRenamesOneWinsOneGetsStaleConflict(t *testing.T) {
	// Both callers read revision 5 before racing. The store commits one
	// (now at revision 6); the loser's attempt, decided against the new
	// current revision, must get a stale-revision conflict, never a silent
	// overwrite.
	winner := RenameAttempt{IdempotencyKey: "kA", RequestDigest: "dA", ExpectedRevision: 5}
	loser := RenameAttempt{IdempotencyKey: "kB", RequestDigest: "dB", ExpectedRevision: 5}

	outcome, _, err := DecideRename(nil, winner, 5)
	if err != nil || outcome != RenameProceed {
		t.Fatalf("winner should proceed, got outcome=%v err=%v", outcome, err)
	}
	// Store now at revision 6 after the winner's commit.
	_, _, err = DecideRename(nil, loser, 6)
	if err == nil || !strings.Contains(err.Error(), "stale revision conflict") {
		t.Fatalf("loser must get a stale-revision conflict, got: %v", err)
	}
}
