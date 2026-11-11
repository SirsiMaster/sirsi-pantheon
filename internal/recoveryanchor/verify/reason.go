package verify

import (
	"fmt"
	"slices"
	"time"
)

// Reason is a stable, machine-readable decision code. Values never change
// meaning once shipped; add new codes, never repurpose one.
type Reason string

// Allow codes.
const (
	ReasonAdmitted    Reason = "admitted"
	ReasonRevalidated Reason = "revalidated"
)

// Deny codes. ADR-078 sections cited in comments.
const (
	// Envelope parsing (§1 "Encoding and namespace").
	ReasonEnvelopeOversize       Reason = "envelope_oversize"
	ReasonEnvelopeMalformed      Reason = "envelope_malformed"
	ReasonEnvelopeDuplicateField Reason = "envelope_duplicate_field"
	ReasonEnvelopeUnknownField   Reason = "envelope_unknown_field"
	ReasonEnvelopeMissingField   Reason = "envelope_missing_field"
	ReasonEnvelopeNonCanonical   Reason = "envelope_non_canonical"

	// Signature container and verification (§1).
	ReasonSignatureOversize  Reason = "signature_oversize"
	ReasonSignatureMalformed Reason = "signature_malformed"
	ReasonWrongNamespace     Reason = "wrong_namespace"
	ReasonKeyTypeNotAllowed  Reason = "key_type_not_allowed"
	ReasonHashNotAllowed     Reason = "hash_not_allowed"
	ReasonSignatureInvalid   Reason = "signature_invalid"
	ReasonUnknownSigner      Reason = "unknown_signer"

	// Epoch (§1a, R6-2).
	ReasonEpochRetired Reason = "epoch_retired"
	ReasonEpochFuture  Reason = "epoch_future"

	// Verifier scope checks (§1a "Scope of the Verifier's checks").
	ReasonUIDMismatch       Reason = "uid_mismatch"
	ReasonWrongNode         Reason = "wrong_node"
	ReasonVerbUnknown       Reason = "verb_unknown"
	ReasonVerbNotConfigured Reason = "verb_not_configured"
	ReasonTargetMismatch    Reason = "target_mismatch"
	ReasonDigestMismatch    Reason = "config_digest_mismatch"
	ReasonTTLInvalid        Reason = "ttl_invalid"
	ReasonTTLTooLong        Reason = "ttl_too_long"
	ReasonNotYetValid       Reason = "not_yet_valid"
	ReasonExpired           Reason = "expired"

	// Authorizer (§2).
	ReasonVerbNotGranted Reason = "verb_not_granted"

	// Atomic admission, ratchet, mark (§1a).
	ReasonClockRollback  Reason = "clock_rollback"
	ReasonReplaySame     Reason = "replay_same_content"
	ReasonReplayConflict Reason = "replay_conflicting_content"
	ReasonBehindMark     Reason = "behind_high_water_mark"
	ReasonStoreError     Reason = "store_error"

	// Policy state and pre-dispatch revalidation (§1a fence, dirty state).
	ReasonPolicyInvalid   Reason = "policy_invalid"
	ReasonPolicyDirty     Reason = "policy_dirty"
	ReasonGenerationMoved Reason = "generation_moved"
	ReasonGrantRevoked    Reason = "grant_revoked"

	// Fail-closed catch-all (bad configuration, recovered panic).
	ReasonInternal Reason = "internal_error"
)

var denyReasons = []Reason{
	ReasonEnvelopeOversize, ReasonEnvelopeMalformed, ReasonEnvelopeDuplicateField,
	ReasonEnvelopeUnknownField, ReasonEnvelopeMissingField, ReasonEnvelopeNonCanonical,
	ReasonSignatureOversize, ReasonSignatureMalformed, ReasonWrongNamespace,
	ReasonKeyTypeNotAllowed, ReasonHashNotAllowed, ReasonSignatureInvalid, ReasonUnknownSigner,
	ReasonEpochRetired, ReasonEpochFuture,
	ReasonUIDMismatch, ReasonWrongNode, ReasonVerbUnknown, ReasonVerbNotConfigured,
	ReasonTargetMismatch, ReasonDigestMismatch, ReasonTTLInvalid, ReasonTTLTooLong,
	ReasonNotYetValid, ReasonExpired, ReasonVerbNotGranted,
	ReasonClockRollback, ReasonReplaySame, ReasonReplayConflict, ReasonBehindMark, ReasonStoreError,
	ReasonPolicyInvalid, ReasonPolicyDirty, ReasonGenerationMoved, ReasonGrantRevoked,
	ReasonInternal,
}

// DenyReasons lists every deny code this package can return.
func DenyReasons() []Reason { return slices.Clone(denyReasons) }

// DenyError is the error form of a deny decision. Detail never contains
// secrets (no key material, no signature bytes).
type DenyError struct {
	Reason Reason
	Detail string
}

func (e *DenyError) Error() string { return string(e.Reason) + ": " + e.Detail }

func deny(r Reason, format string, a ...any) *DenyError {
	return &DenyError{Reason: r, Detail: fmt.Sprintf(format, a...)}
}

// Admitted is the record of a request that cleared admission. It is the
// input to Revalidate. It carries no authority by itself.
type Admitted struct {
	Envelope    Envelope
	Fingerprint string // signing key, "SHA256:..." form
	UID         uint32 // presenting UID, equal to the key's mapped UID
	ContentHash [32]byte
	Generation  uint64 // policy generation it was authorized under
	AdmittedAt  time.Time
}

// Decision is the typed result of Admit and Revalidate. Allowed is true only
// for ReasonAdmitted / ReasonRevalidated. A Decision is advice to a caller
// that may later act; this package never acts.
type Decision struct {
	Allowed  bool
	Reason   Reason
	Detail   string
	Admitted *Admitted // non-nil only for an Admit allow
}

func denied(r Reason, detail string) Decision {
	return Decision{Reason: r, Detail: detail}
}

func deniedErr(err error) Decision {
	if de, ok := err.(*DenyError); ok {
		return denied(de.Reason, de.Detail)
	}
	return denied(ReasonInternal, "unclassified error")
}
