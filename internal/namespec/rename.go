package namespec

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// RequestDigest computes the ADR-072 C4 request digest: sha256 over the
// RFC 8785 JCS-canonicalized bytes of raw, which must be the JSON encoding of
// the actual requested change (e.g. thread-id, old name, new name) -- never
// including the idempotency key itself, since the key and the digest of what
// it was used for are compared separately (DecideRename).
func RequestDigest(raw []byte) (string, error) {
	canon, err := canonicalize(raw)
	if err != nil {
		return "", fmt.Errorf("namespec: canonicalize request for digest: %w", err)
	}
	sum := sha256.Sum256(canon)
	return hex.EncodeToString(sum[:]), nil
}

// IdempotencyRecord is a prior commit stored under one idempotency key: the
// digest of the request that produced it, and the receipt to return on an
// exact replay. The store resolves this by key lookup; namespec never reads
// the store itself.
type IdempotencyRecord struct {
	Digest  string
	Receipt string
}

// RenameAttempt is one rename request's caller-presented facts: the
// idempotency key, the digest of the actual requested change (RequestDigest
// over the payload, not the key string), and the mapping revision the caller
// read before issuing the request.
type RenameAttempt struct {
	IdempotencyKey   string
	RequestDigest    string
	ExpectedRevision int64
}

// RenameOutcome is the four-way ADR-072 C4 verdict for a rename attempt.
type RenameOutcome int

const (
	// RenameProceed means no prior idempotency record exists and the caller's
	// expected revision matches the current one: the CAS may commit.
	RenameProceed RenameOutcome = iota
	// RenameReplay means a prior record exists under this key with the same
	// digest: return its receipt unchanged, apply nothing. This is checked
	// BEFORE the CAS comparison, so a request that lands after its own
	// commit (slow reply, crash-after-commit) is a replay, never a conflict.
	RenameReplay
	// RenameRejectKeyReuse means a prior record exists under this key with a
	// DIFFERENT digest: the same key was reused for a different payload.
	// Rejected outright -- never applied as a new rename, never treated as a
	// replay of the old one.
	RenameRejectKeyReuse
	// RenameStaleRevisionConflict means no prior record exists for this key,
	// but the caller's expected revision no longer matches the current one:
	// another rename committed first (the CAS loser).
	RenameStaleRevisionConflict
)

// DecideRename is the ADR-072 C4 pure decision for one rename attempt, given
// prior (the IdempotencyRecord stored under attempt.IdempotencyKey, nil if
// none) and currentRevision (the mapping's current revision at decision
// time). It never touches a store; the caller supplies both facts and
// commits the outcome inside its own transaction.
//
// Idempotency is checked before concurrency, matching the ADR: a same-
// key-same-digest replay returns RenameReplay (with the original receipt)
// even if the current revision has already moved past
// attempt.ExpectedRevision, because that movement is this request's own
// prior commit, not a conflicting one.
//
// replayReceipt is only meaningful when the outcome is RenameReplay; it is
// prior.Receipt, returned unchanged.
func DecideRename(prior *IdempotencyRecord, attempt RenameAttempt, currentRevision int64) (outcome RenameOutcome, replayReceipt string, err error) {
	if prior != nil {
		if prior.Digest == attempt.RequestDigest {
			return RenameReplay, prior.Receipt, nil
		}
		return 0, "", fmt.Errorf("namespec: idempotency key %q was already used for a different request (rejected, not applied as a new rename and not treated as a replay)", attempt.IdempotencyKey)
	}
	if attempt.ExpectedRevision != currentRevision {
		return 0, "", fmt.Errorf("namespec: stale revision conflict on rename: expected revision %d, current revision is %d", attempt.ExpectedRevision, currentRevision)
	}
	return RenameProceed, "", nil
}
