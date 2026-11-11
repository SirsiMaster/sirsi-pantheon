package schedule

// Waiter-first admission (ADR-120 amendment 2026-11-10 part 2; ledger task
// adr120-s3-08-exclusive-cooldown; owner directive "forgiving sharing" part 5:
// no back-to-back exclusive holds by one lane on the same resource).
//
// The rule is not a timer. When a request would be granted the FULL share and a
// lane that QUEUED behind this very holder is still waiting on the resource,
// the requester is not granted full: a --queue request lines up behind the
// waiter, any other is refused (ErrWaiterFirst, nothing saved). It is never
// floor-granted — a floor grant beside nobody would be a silent downgrade, and
// quiet is never floor-granted.
//
// KNOWN LIMIT: the rule protects only a lane that QUEUES. A lane that polls for
// "clear", takes a floor grant, or gives up is invisible to it. A queued row is
// also never heartbeated (Heartbeat refuses non-active rows, and `reserve
// --queue` returns at once), so a waiter is protected only for its row's lease
// (LeaseTTLSec, default 120 s from queueing) unless it queued with a longer
// --lease-ttl or re-queues.

import (
	"errors"
	"fmt"
	"time"
)

// waiterBound is the owner's standing 120-second number for how long a queued
// lane may wait before the refusal is labeled waiter-bound rather than
// waiter-first. Waiter-first already refuses a repeater for as long as a
// qualifying waiter is live; the bound only changes the reason code.
const waiterBound = 120 * time.Second

// Reason codes carried on ReserveResult.ReasonCode and WaiterFirstError.
const (
	ReasonWaiterFirst = "waiter-first"
	ReasonWaiterBound = "waiter-bound"
)

// ErrWaiterFirst is matched (errors.Is) by every waiter-first refusal, locally
// and across the remote store boundary (registered in routerstore's wire table).
var ErrWaiterFirst = errors.New("maat: a waiting lane goes first")

// WaiterInfo names the lane that goes next.
type WaiterInfo struct {
	Holder   string `json:"holder"`
	Work     string `json:"work"`
	QueuedAt string `json:"queued_at"` // RFC3339 of the waiter row's Created
}

// WaiterFirstError is the refusal. It carries the structured detail so a CLI
// can print JSON; its text deliberately contains none of the transport-error
// phrases a guard retries on.
type WaiterFirstError struct {
	Resource   string     `json:"resource"`
	ReasonCode string     `json:"reason_code"`
	Waiter     WaiterInfo `json:"waiter"`
}

func newWaiterFirstError(resource, code string, w WaiterInfo) *WaiterFirstError {
	return &WaiterFirstError{Resource: resource, ReasonCode: code, Waiter: w}
}

func (e *WaiterFirstError) Error() string {
	return fmt.Sprintf("maat: %s has been waiting for %s since %s and goes next (%s) — no new full hold here until it has run; re-run with --queue to line up behind it",
		e.Waiter.Holder, e.Resource, e.Waiter.QueuedAt, e.ReasonCode)
}

func (e *WaiterFirstError) Is(target error) bool { return target == ErrWaiterFirst }

// waiterFor returns the OLDEST waiter the requester owes a turn to, or nil: a
// StatusQueued row on the same resource, a different holder, a live lease, a
// window overlapping the request, that queued behind req.Holder. Rows queued
// before QueuedBehind existed read as empty and never match. A waiter that has
// since been served (the same holder later holds a non-floor row created at or
// after its queueing) no longer counts — its stale queued row must not penalize
// the lane it already got its turn from.
func waiterFor(rows []Reservation, req Reservation, now time.Time) *Reservation {
	var oldest *Reservation
	var oldestAt time.Time
	for i := range rows {
		w := rows[i]
		if w.Status != StatusQueued || w.Holder == req.Holder || w.QueuedBehind != req.Holder ||
			!sameResource(w.Resource, req.Resource) || w.leaseExpired(now) || !w.windowOverlaps(req) {
			continue
		}
		at, err := time.Parse(time.RFC3339, w.Created)
		if err != nil || waiterServed(rows, w, at) {
			continue
		}
		if oldest == nil || at.Before(oldestAt) {
			oldest, oldestAt = &rows[i], at
		}
	}
	return oldest
}

func waiterServed(rows []Reservation, w Reservation, queuedAt time.Time) bool {
	for _, s := range rows {
		if s.ID == w.ID || s.Holder != w.Holder || s.Status == StatusQueued || s.Share == ShareFloor || !sameResource(s.Resource, w.Resource) {
			continue
		}
		if c, err := time.Parse(time.RFC3339, s.Created); err == nil && !c.Before(queuedAt) {
			return true
		}
	}
	return false
}

func waiterReason(w Reservation, now time.Time) (string, WaiterInfo) {
	code := ReasonWaiterFirst
	if at, err := time.Parse(time.RFC3339, w.Created); err == nil && now.Sub(at) > waiterBound {
		code = ReasonWaiterBound
	}
	return code, WaiterInfo{Holder: w.Holder, Work: w.Work, QueuedAt: w.Created}
}
