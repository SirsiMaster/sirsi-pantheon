package routerstore

// DeliveryState is a DERIVED, read-only label that maps an item's existing
// status + acked_at onto A2A-style task-state names (ADR-065 Decision 2, as
// corrected by SSA: a delivery attempt is distinct from a read-ack, and only
// the lane's own `router acknowledge` writes acked_at). It borrows A2A
// vocabulary only, not the wire protocol, and adds no storage: it is a pure
// function of fields Item already carries.
type DeliveryState string

const (
	// DeliverySubmitted: status open, never acknowledged. Proves only that the
	// item is in the ledger. It does NOT prove the lane saw it: a wake attempt
	// (WakeStatus/WakeAdapter) is a delivery attempt, never a read-ack.
	DeliverySubmitted DeliveryState = "SUBMITTED"
	// DeliveryAcknowledged: status open, acked_at set. Proves the lane's own
	// `router acknowledge` ran, i.e. the lane read the body. It does NOT prove
	// the lane will act; it is also what an item looks like after a failed
	// attempt below the retry ceiling returns it to open (acked_at survives).
	DeliveryAcknowledged DeliveryState = "ACKNOWLEDGED"
	// DeliveryWorking: status claimed or working. Proves a lease was taken (the
	// only door to execution). It does NOT prove the lease is still live: Item
	// carries no lease fields, so an expired lease reads WORKING until the next
	// claim reclaims it (lease.go claim path step 1). It also cannot tell
	// claimed from working; both mean "someone holds it".
	DeliveryWorking DeliveryState = "WORKING"
	// DeliveryBlocked: status blocked. Proves a worker parked it on an
	// external/owner dependency (lease.go Block). Deliberately NOT mapped to
	// INPUT_REQUIRED/AUTH_REQUIRED: the data cannot say whose input is missing.
	DeliveryBlocked DeliveryState = "BLOCKED"
	// DeliveryCompleted: status completed. Proves a lease holder called Complete
	// with a result. It does NOT prove the result is correct or reviewed.
	DeliveryCompleted DeliveryState = "COMPLETED"
	// DeliveryFailed: status dead_letter. Proves the retry ceiling was spent (or
	// an owner force-dead-lettered it). It does NOT say why; see failure_class.
	DeliveryFailed DeliveryState = "FAILED"
	// DeliveryClosed: legacy status closed (file-router vocabulary, CloseItem).
	// Terminal, but the outcome is unknown: closed is used for done, withdrawn
	// and superseded alike, so it is NOT mapped to COMPLETED.
	DeliveryClosed DeliveryState = "CLOSED"
	// DeliveryUnknown: a status this mapping does not know. Never guessed.
	DeliveryUnknown DeliveryState = "UNKNOWN"
)

// DeliveryStateOf derives the delivery state of an item.
//
//	status        acked_at  -> state
//	open          ""        -> SUBMITTED
//	open          set       -> ACKNOWLEDGED
//	claimed       any       -> WORKING   (acked_at does not change a held lease)
//	working       any       -> WORKING
//	blocked       any       -> BLOCKED
//	completed     any       -> COMPLETED
//	dead_letter   any       -> FAILED
//	closed        any       -> CLOSED
//	anything else any       -> UNKNOWN
func DeliveryStateOf(it Item) DeliveryState {
	switch it.Status {
	case StatusOpen:
		if it.AckedAt == "" {
			return DeliverySubmitted
		}
		return DeliveryAcknowledged
	case StatusClaimed, StatusWorking:
		return DeliveryWorking
	case StatusBlocked:
		return DeliveryBlocked
	case StatusCompleted:
		return DeliveryCompleted
	case StatusDeadLetter:
		return DeliveryFailed
	case StatusClosed:
		return DeliveryClosed
	}
	return DeliveryUnknown
}
