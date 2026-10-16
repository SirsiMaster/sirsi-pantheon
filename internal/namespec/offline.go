package namespec

import "fmt"

// OperationKind classifies a registry operation for the ADR-072 C2 offline
// carve-out. A disconnected host's last-verified promotion receipt proves
// only that a schema was once admitted -- it is never upgraded into global
// mutation authority, so the carve-out distinguishes operations that stay
// within the already-admitted read-only surface from ones that would mutate
// the global 1:1 namespace.
type OperationKind int

const (
	// OpReadOnly covers validating an already-held name's shape, answering
	// `sirsi router doctor`-style local diagnostics, and serving previously
	// cached lookups -- the only surface a disconnected host may serve.
	OpReadOnly OperationKind = iota
	// OpMutating covers new registration, rename, reuse/promotion out of
	// tombstone, and schema promotion -- anything that mutates the global
	// namespace. Always refused while disconnected, full stop; there is no
	// fenced delegated-writer mode (ADR-072 C2).
	OpMutating
)

func (k OperationKind) String() string {
	if k == OpMutating {
		return "mutating"
	}
	return "read-only"
}

// OfflineCredential is what the caller (the router service, which owns the
// ADR-067 machine-id credential store) reports about the presenting host's
// credential. namespec has no access to that store itself and trusts only
// what the caller supplies here.
type OfflineCredential struct {
	// Revoked is true when the host's machine-id credential (ADR-067) has
	// been revoked since the last-verified promotion receipt was obtained.
	Revoked bool
}

// AuthorizeOffline is the ADR-072 C2 offline-carve-out decision: whether a
// disconnected host, holding receipt as its last-verified promotion
// receipt, may perform op under cred.
//
// It fails closed: a missing/never-verified receipt or a revoked credential
// refuses even a read-only operation, distinctly from a merely-stale-schema
// refusal, and a mutating operation is refused unconditionally regardless of
// receipt or credential state. Reconnection is required for any mutation:
// AuthorizeOffline governs only while disconnected. Once reachable,
// ValidateAgainstActive governs instead, with no special-cased
// "offline-origin" bypass for a request that was queued while disconnected
// and is now replaying -- it is subject to the same checks as any other
// request.
func AuthorizeOffline(receipt *PromotionReceipt, cred OfflineCredential, op OperationKind) error {
	if receipt == nil {
		return fmt.Errorf("namespec: no last-verified promotion receipt held -- offline %s operation refused (fail closed)", op)
	}
	if cred.Revoked {
		return fmt.Errorf("namespec: machine-id credential is revoked -- offline %s operation refused, not merely stale-schema-refused", op)
	}
	if op == OpMutating {
		return fmt.Errorf("namespec: mutating operations are never authorized while disconnected (offline carve-out is read-only only)")
	}
	return nil
}
