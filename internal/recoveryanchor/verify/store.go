package verify

import "time"

// Clock is the injected wall clock. The ratchet exists because this value can
// be wrong; nothing here reads time.Now directly.
type Clock interface{ Now() time.Time }

// ClockFunc adapts a function to Clock.
type ClockFunc func() time.Time

// Now implements Clock.
func (f ClockFunc) Now() time.Time { return f() }

// Store is the durable admission state. A real implementation (not part of
// this package, and not authorized yet) MUST provide:
//
//   - Serializability: Update calls behave as if executed one at a time
//     (ADR-078 §1a "per-node serialization lock"). Two concurrent admissions
//     of the same (fingerprint, request_id) must never both Reserve.
//   - Atomicity and durability: every effect of fn is applied together, or
//     none is, and Update returns nil only after the effects are durable
//     (fsync'd). If fn returns an error, nothing it wrote persists. A crash
//     mid-Update leaves the pre-Update state.
//   - Epoch scoping: Ratchet, SetRatchet, Mark, SetMark address the CURRENT
//     epoch only; StartEpoch retains the retired epoch's values, unmutated,
//     for audit.
//
// This package's own logic performs all reads before any write inside fn and
// writes only when it is about to allow, so a deny never changes state.
type Store interface {
	Update(fn func(Tx) error) error
}

// Tx is the view of the store inside one Update. Times are Unix seconds.
type Tx interface {
	// Epoch is the daemon's current protected epoch (>= 1).
	Epoch() uint64
	// Ratchet is last_observed_wallclock of the current epoch.
	Ratchet() int64
	SetRatchet(unix int64)
	// Mark is the admission-order high-water mark of the current epoch.
	Mark() int64
	SetMark(unix int64)
	// Reservation looks up UNIQUE(fingerprint, request_id) and returns the
	// stored sha256 of the canonical envelope bytes.
	Reservation(fingerprint, requestID string) (contentHash [32]byte, found bool)
	Reserve(fingerprint, requestID string, contentHash [32]byte, expiresAt int64)
	// Reservations lists every row, for the pruning routine.
	Reservations() []ReservationInfo
	DeleteReservation(fingerprint, requestID string)
	// StartEpoch mints epoch+1 with a freshly initialized ratchet and mark,
	// retaining the retired epoch's state. Returns the new epoch.
	StartEpoch(ratchet, mark int64) uint64
}

// ReservationInfo describes one replay row.
type ReservationInfo struct {
	Fingerprint string
	RequestID   string
	ExpiresAt   int64
}

func skewSeconds(d time.Duration) int64 { return int64(d / time.Second) }

// rolledBack is the ratchet test, identical at admission, startup, tick and
// pre-dispatch: the clock reads behind the ratchet minus max skew.
func rolledBack(nowUnix, ratchet int64, maxSkew time.Duration) bool {
	return nowUnix < ratchet-skewSeconds(maxSkew)
}

func advanceRatchet(tx Tx, nowUnix int64) {
	if nowUnix > tx.Ratchet() {
		tx.SetRatchet(nowUnix)
	}
}

// State is a read-only snapshot of the current epoch, for callers assembling
// a Current for Revalidate inside their own guard.
type State struct {
	Epoch   uint64
	Ratchet int64
	Mark    int64
}

// ReadState reads the current epoch's state without writing.
func ReadState(s Store) (State, error) {
	var st State
	err := s.Update(func(tx Tx) error {
		st = State{Epoch: tx.Epoch(), Ratchet: tx.Ratchet(), Mark: tx.Mark()}
		return nil
	})
	if err != nil {
		return State{}, deny(ReasonStoreError, "store read failed")
	}
	return st, nil
}

// Tick is the ratchet's periodic durable advance (and the startup step): it
// runs the rollback check, then advances the ratchet to max(current, now),
// even with zero admissions in flight. On a detected rollback it returns a
// *DenyError{ReasonClockRollback} and leaves the ratchet unchanged: the
// caller must fail closed (deny all) until the clock recovers or an
// owner-gated StartEpoch.
func Tick(s Store, c Clock, maxSkew time.Duration) error {
	now := c.Now().Unix()
	var de *DenyError
	err := s.Update(func(tx Tx) error {
		if rolledBack(now, tx.Ratchet(), maxSkew) {
			de = deny(ReasonClockRollback, "clock reads behind ratchet minus max skew")
			return nil
		}
		advanceRatchet(tx, now)
		return nil
	})
	if err != nil {
		return deny(ReasonStoreError, "store update failed")
	}
	if de != nil {
		return de
	}
	return nil
}

// Prune deletes replay rows only once expires_at + maxTTL + maxSkew has
// passed, and in the SAME transaction advances the ratchet to the trusted
// wall-clock observation taken now (never to a deleted row's own expires_at:
// ADR-078 §1a, R6-3). It refuses to prune while a rollback is detected,
// since the observation would not be trustworthy. Returns rows deleted.
func Prune(s Store, c Clock, maxTTL, maxSkew time.Duration) (int, error) {
	now := c.Now().Unix()
	floor := skewSeconds(maxTTL) + skewSeconds(maxSkew)
	deleted := 0
	var de *DenyError
	err := s.Update(func(tx Tx) error {
		if rolledBack(now, tx.Ratchet(), maxSkew) {
			de = deny(ReasonClockRollback, "refusing to prune under a detected clock rollback")
			return nil
		}
		for _, r := range tx.Reservations() {
			if r.ExpiresAt+floor <= now {
				tx.DeleteReservation(r.Fingerprint, r.RequestID)
				deleted++
			}
		}
		advanceRatchet(tx, now)
		return nil
	})
	if err != nil {
		return 0, deny(ReasonStoreError, "store update failed")
	}
	if de != nil {
		return 0, de
	}
	return deleted, nil
}

// StartEpoch is the owner-gated override transaction (ADR-078 §1a): mint the
// next protected epoch and initialize that epoch's ratchet and mark from one
// trusted wall-clock observation, atomically. The retired epoch's values are
// retained, and every envelope signed under it stays rejected at the epoch
// check. THIS LIBRARY DOES NOT AUTHENTICATE THE OWNER: the caller must gate
// this call on the owner decision the ADR requires; it is never a self-heal.
func StartEpoch(s Store, c Clock) (uint64, error) {
	now := c.Now().Unix()
	var epoch uint64
	err := s.Update(func(tx Tx) error {
		epoch = tx.StartEpoch(now, now)
		return nil
	})
	if err != nil {
		return 0, deny(ReasonStoreError, "store update failed")
	}
	return epoch, nil
}
