package schedule

// `reserve --queue --wait` (ledger task maat-reserve-queue-wait). A queued row
// used to be a promise nobody kept: it was never heartbeated (so it protected its
// lane only for one lease), and nothing ever turned it into a grant. The waiter
// now stays attached and polls Claim, which in ONE server-side admission pass
// both refreshes the row's lease (the heartbeat) and, if the resource has freed,
// replaces the queued row with the grant. No daemon: if the waiting process dies
// the heartbeats stop and the row stops counting as a waiter (queuedLive) within
// one lease.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotQueued: the row a Claim named is gone, or no longer queued or granted
// (released, expired, invalidated). Registered on the wire in routerstore.
var ErrNotQueued = errors.New("maat: reservation is not queued")

// ErrWaitTimeout is returned (wrapped) when --wait elapsed with no grant; the
// result carries ReasonCode ReasonWaitTimeout and the queued row was canceled.
var ErrWaitTimeout = errors.New("maat: wait timed out")

// ReasonWaitTimeout is the reason_code of a timed-out wait.
const ReasonWaitTimeout = "wait-timeout"

// startNow moves a window that began before now (the wait elapsed) to begin now,
// keeping its length, so a grant never starts with its window already spent.
func startNow(r Reservation, now time.Time) Reservation {
	s, e := parseWindow(r)
	if !s.Before(now) {
		return r
	}
	r.Start = now.Format(time.RFC3339)
	if r.EstEnd != "" && e.After(s) {
		r.EstEnd = now.Add(e.Sub(s)).Format(time.RFC3339)
	}
	return r
}

// Claim is one atomic admission attempt for the queued row id. It returns the
// row: StatusActive when granted (a repeat Claim of a granted row returns the
// same grant, so a lost reply loses nothing), StatusQueued when it must keep
// waiting (lease refreshed). The decision is made at the canonical store in the
// same pass that writes it; the caller never checks then acts.
func (l *Ledger) Claim(id string) (*Reservation, error) {
	if a, ok := l.store.(AtomicScheduler); ok {
		return a.MaatMutate("claim", id, "")
	}
	rows, err := l.ExactReservation(id)
	if err != nil {
		return nil, err
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("%w: %s not found", ErrNotQueued, id)
	}
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		res, err := l.reserveAttempt(rows[0], true, true)
		if errors.Is(err, errLedgerConflict) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return res.Reservation, nil
	}
	return nil, fmt.Errorf("maat: claim: contention limit: %w", errLedgerConflict)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// ReserveWait queues behind the holder (or is granted at once) and then stays
// attached for up to wait, returning the grant as soon as the resource frees.
// On timeout or cancellation the queued row is released and the error says why.
func (l *Ledger) ReserveWait(ctx context.Context, req Reservation, wait time.Duration) (ReserveResult, error) {
	return l.reserveWait(ctx, req, wait, sleepCtx)
}

func (l *Ledger) reserveWait(ctx context.Context, req Reservation, wait time.Duration, sleep func(context.Context, time.Duration) error) (ReserveResult, error) {
	if wait <= 0 {
		return ReserveResult{}, errors.New("maat: --wait must be positive")
	}
	if req.ID == "" {
		id, err := l.NewReservationID(req.Resource, req.Holder)
		if err != nil {
			return ReserveResult{}, err
		}
		req.ID = id
	}
	res, err := l.Reserve(req, true)
	if err != nil {
		// The id is ours, so a lost reply is recoverable: the row may exist.
		rows, qerr := l.ExactReservation(req.ID)
		if qerr != nil || len(rows) != 1 || rows[0].Holder != req.Holder || !sameResource(rows[0].Resource, req.Resource) {
			return ReserveResult{}, err
		}
		row := rows[0]
		switch row.Status {
		case StatusActive:
			return ReserveResult{Granted: true, Reservation: &row}, nil
		case StatusQueued:
			res = ReserveResult{Queued: true, Reservation: &row}
		default:
			return ReserveResult{}, err
		}
	}
	if res.Granted || !res.Queued {
		return res, nil
	}
	id := res.Reservation.ID
	// Poll well inside the lease so a live waiter never lapses, but never hammer.
	poll := min(max(time.Duration(res.Reservation.LeaseTTLSec)*time.Second/3, 100*time.Millisecond), 5*time.Second)
	deadline := l.now().Add(wait)
	cancel := func() { _, _ = l.Release(id) }
	var lastErr error
	for {
		if d := min(poll, deadline.Sub(l.now())); d > 0 {
			if err := sleep(ctx, d); err != nil {
				cancel()
				return ReserveResult{}, fmt.Errorf("maat: wait canceled: %w", err)
			}
		}
		row, err := l.Claim(id)
		switch {
		case err == nil && row.Status == StatusActive:
			return ReserveResult{Granted: true, Reservation: row}, nil
		case errors.Is(err, ErrNotQueued):
			return ReserveResult{}, err
		case err != nil && strings.Contains(err.Error(), "unknown Ma'at mutation"):
			cancel()
			return ReserveResult{}, fmt.Errorf("maat: this router service does not support --wait (claim); upgrade it: %w", err)
		}
		lastErr = err // transient: keep trying, the lease bounds the exposure
		if !l.now().Before(deadline) {
			break
		}
	}
	cancel()
	out := ReserveResult{ReasonCode: ReasonWaitTimeout, Conflict: res.Conflict, Waiter: res.Waiter}
	if lastErr != nil {
		return out, fmt.Errorf("%w after %s waiting for %s (last claim error: %v)", ErrWaitTimeout, wait, req.Resource, lastErr)
	}
	return out, fmt.Errorf("%w after %s waiting for %s", ErrWaitTimeout, wait, req.Resource)
}
