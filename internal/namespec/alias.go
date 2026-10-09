package namespec

import "fmt"

// NameState is the ADR-072 C5 state of one bound name slot.
type NameState int

const (
	// StateCanonical is the live holder's current name.
	StateCanonical NameState = iota
	// StateActiveAlias is a prior name of the same still-live thread-id,
	// during its migration window: mail to it redirects to the canonical
	// name, unconditionally.
	StateActiveAlias
	// StateExpiredAlias is an active alias whose migration window has been
	// explicitly ended (or elapsed) while its thread-id is still live
	// elsewhere under its canonical name. Compatibility delivery stops.
	StateExpiredAlias
	// StateTombstone is a prior name of a now-retired thread-id, retained
	// for redirect/status, never for delivery. It may later be superseded
	// (Reused) by a new holder under a new generation.
	StateTombstone
)

func (s NameState) String() string {
	switch s {
	case StateCanonical:
		return "canonical"
	case StateActiveAlias:
		return "active-alias"
	case StateExpiredAlias:
		return "expired-alias"
	case StateTombstone:
		return "tombstone"
	default:
		return fmt.Sprintf("unknown-state-%d", int(s))
	}
}

// NameRecord is the pure C5 state of one bound name slot, as resolved by the
// caller (the router service) from its store. namespec never reads the store
// itself.
type NameRecord struct {
	State NameState
	// ThreadID is the thread this record currently resolves to for delivery:
	// for Canonical/ActiveAlias/ExpiredAlias, the live thread the name (or
	// its alias) belongs to; for a Reused Tombstone, the new holder's
	// thread-id; for a not-yet-reused Tombstone, empty.
	ThreadID string
	// Generation is the name slot's current generation number.
	Generation int
	// CanonicalName is the live thread's current canonical name -- set for
	// ActiveAlias and ExpiredAlias (the redirect/refusal target).
	CanonicalName string
	// Successor is an optional known successor name for a not-yet-reused
	// Tombstone (set only once a reassignment has already named one).
	Successor string
	// Reused is true when a Tombstone has been superseded by a new
	// generation (a new holder claimed the name); meaningless otherwise.
	Reused bool
}

// DeliveryStatus is the structured outcome ResolveDelivery returns -- every
// value corresponds 1:1 to a protocol response shape the router sends back,
// never a Go usage error.
type DeliveryStatus int

const (
	// DeliveryOK: deliver to ThreadID at the current Generation.
	DeliveryOK DeliveryStatus = iota
	// DeliveryRenamed: {status: renamed, to, thread_id, generation} -- the
	// redirect target's own durable identity, so a sender who follows it
	// later can detect whether <to> has since been superseded by reuse.
	DeliveryRenamed
	// DeliveryAliasExpired: {status: alias-expired, canonical, thread_id,
	// generation} -- compatibility delivery has stopped; never redirected.
	DeliveryAliasExpired
	// DeliveryRetired: {status: retired, successor?}.
	DeliveryRetired
	// DeliveryStale: {status: stale, expected_generation, current} -- a
	// presented generation that no longer matches the name slot's current
	// one, never silently redirected into an unrelated new holder's inbox.
	DeliveryStale
	// DeliveryAmbiguous: a legacy name-only send (no resolved generation)
	// against a name slot with more than one generation on record. The
	// router never guesses which generation a bare name means once more
	// than one has existed.
	DeliveryAmbiguous
)

// DeliveryDecision is ResolveDelivery's result. Only the fields relevant to
// Status are meaningful; the rest are zero.
type DeliveryDecision struct {
	Status             DeliveryStatus
	ThreadID           string
	Generation         int
	To                 string
	Canonical          string
	Successor          string
	ExpectedGeneration int
	CurrentGeneration  int
}

// ResolveDelivery is the ADR-072 C5 pure mail-routing decision for a message
// addressed to a name whose current slot state is record. presentedGeneration
// is the sender's resolved expected generation, or nil for a legacy
// name-only send that never resolved one.
func ResolveDelivery(record NameRecord, presentedGeneration *int) (DeliveryDecision, error) {
	switch record.State {
	case StateCanonical:
		if presentedGeneration != nil && *presentedGeneration != record.Generation {
			return DeliveryDecision{Status: DeliveryStale, ExpectedGeneration: *presentedGeneration, CurrentGeneration: record.Generation}, nil
		}
		return DeliveryDecision{Status: DeliveryOK, ThreadID: record.ThreadID, Generation: record.Generation}, nil
	case StateActiveAlias:
		return DeliveryDecision{Status: DeliveryRenamed, To: record.CanonicalName, ThreadID: record.ThreadID, Generation: record.Generation}, nil
	case StateExpiredAlias:
		return DeliveryDecision{Status: DeliveryAliasExpired, Canonical: record.CanonicalName, ThreadID: record.ThreadID, Generation: record.Generation}, nil
	case StateTombstone:
		if !record.Reused {
			return DeliveryDecision{Status: DeliveryRetired, Successor: record.Successor}, nil
		}
		if presentedGeneration == nil {
			return DeliveryDecision{Status: DeliveryAmbiguous}, nil
		}
		if *presentedGeneration != record.Generation {
			return DeliveryDecision{Status: DeliveryStale, ExpectedGeneration: *presentedGeneration, CurrentGeneration: record.Generation}, nil
		}
		return DeliveryDecision{Status: DeliveryOK, ThreadID: record.ThreadID, Generation: record.Generation}, nil
	default:
		return DeliveryDecision{}, fmt.Errorf("namespec: unknown name state %d", int(record.State))
	}
}

// AuthorizeReuse is the C5 collision policy: a name can be promoted from
// tombstone back to canonical (reuse, under a new generation) only when its
// live row is currently a tombstone -- never while it is canonical, an
// active alias, or an expired alias.
func AuthorizeReuse(current NameState) error {
	if current != StateTombstone {
		return fmt.Errorf("namespec: name cannot be reused while its live row is %s (reuse is legal only from a tombstone)", current)
	}
	return nil
}
