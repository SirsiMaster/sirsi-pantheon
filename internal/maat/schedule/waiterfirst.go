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
// (LeaseTTLSec, default 120 s from queueing, capped at maxQueuedLeaseSec) or
// until it re-queues, pending a `reserve --queue --wait`.

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
// since been served (waiterServed) no longer counts.
//
// Precedence (breaks every cycle): a candidate does NOT count against the
// requester if the requester has its own live, unserved queued row on the same
// resource that is OLDER than the candidate (Created, ties by ID). The requester
// was waiting first, so it is the owed turn, not the repeater. In any cycle of
// mutual QueuedBehind the oldest queued row's holder always finds every
// candidate younger than its own row and is grantable.
func waiterFor(rows []Reservation, req Reservation, now time.Time) *Reservation {
	var oldest *Reservation
	var oldestAt time.Time
	for i := range rows {
		w := rows[i]
		if w.Status != StatusQueued || w.Holder == req.Holder || w.QueuedBehind != req.Holder ||
			!sameResource(w.Resource, req.Resource) || !w.queuedLive(now) || !w.windowOverlaps(req) {
			continue
		}
		at, err := time.Parse(time.RFC3339, w.Created)
		if err != nil || waiterServed(rows, w, at) || ownOlderQueued(rows, req, w, at, now) {
			continue
		}
		if oldest == nil || at.Before(oldestAt) {
			oldest, oldestAt = &rows[i], at
		}
	}
	return oldest
}

// ownOlderQueued reports whether req.Holder has a live, unserved queued row on
// w's resource that precedes w (earlier Created, ties by smaller ID).
//
// The row must be RELEVANT to this contention (SSA 20261119-051245): its window
// must overlap the part of the request the waiter is owed, i.e. the
// intersection of req's and w's windows. An older queued row for an unrelated
// window waives nothing.
func ownOlderQueued(rows []Reservation, req, w Reservation, wAt, now time.Time) bool {
	rs, re := parseWindow(req)
	ws, we := parseWindow(w)
	is, ie := latest(rs, ws), earliest(re, we) // contested interval [is, ie)
	for _, o := range rows {
		if o.Status != StatusQueued || o.Holder != req.Holder || o.ID == w.ID ||
			!sameResource(o.Resource, w.Resource) || !o.queuedLive(now) {
			continue
		}
		if os, oe := parseWindow(o); !(os.Before(ie) && is.Before(oe)) {
			continue
		}
		oAt, err := time.Parse(time.RFC3339, o.Created)
		if err != nil || !(oAt.Before(wAt) || (oAt.Equal(wAt) && o.ID < w.ID)) || waiterServed(rows, o, oAt) {
			continue
		}
		return true
	}
	return false
}

// waiterServed: the waiter's holder was granted a FULL turn on the resource that
// overlaps the owed window, at or after it queued. Status may be anything but
// queued: a full grant that was later released, finished, expired or
// invalidated by a conflict WAS the turn the waiter was owed. Floor co-runs are
// not the turn; neither are unrelated bookings on a non-overlapping window.
func waiterServed(rows []Reservation, w Reservation, queuedAt time.Time) bool {
	for _, s := range rows {
		if s.ID == w.ID || s.Holder != w.Holder || s.Status == StatusQueued || s.Share != ShareFull ||
			!sameResource(s.Resource, w.Resource) || !s.windowOverlaps(w) {
			continue
		}
		if c, err := time.Parse(time.RFC3339, s.Created); err == nil && !c.Before(queuedAt) {
			return true
		}
	}
	return false
}

// maxQueuedLeaseSec caps a QUEUED row's lease (Mercury review 20261111-065127):
// a lane that queues with a huge --lease-ttl and dies would otherwise hold the
// repeater off the resource for that long. Active rows are unaffected. Lift when
// `reserve --queue --wait` exists.
const maxQueuedLeaseSec = 600

// defaultQueuedLeaseSec is the finite lease a queued row gets when the request
// carries none (zero: the usual default) or a non-positive value.
const defaultQueuedLeaseSec = 120

// capQueuedLease makes a queued row's lease finite and positive: <= 0 becomes
// the 120 s default, > 600 is capped. Both notes APPEND to any existing Reason.
// (leaseExpired treats a non-positive TTL as "never expires", which is right
// for nothing a waiter may rely on.)
func capQueuedLease(r *Reservation) {
	note := ""
	switch {
	case r.LeaseTTLSec <= 0:
		note = fmt.Sprintf("queued lease %d s is not positive; set to %d s", r.LeaseTTLSec, defaultQueuedLeaseSec)
		r.LeaseTTLSec = defaultQueuedLeaseSec
	case r.LeaseTTLSec > maxQueuedLeaseSec:
		note = fmt.Sprintf("queued lease capped at %d s", maxQueuedLeaseSec)
		r.LeaseTTLSec = maxQueuedLeaseSec
	default:
		return
	}
	if r.Reason != "" {
		note = r.Reason + "; " + note
	}
	r.Reason = note
}

// queuedLive: a queued row counts as a waiter only while its lease is positive
// and unexpired. A stored legacy row with TTL <= 0 never counts (it would
// otherwise be immortal).
func (r Reservation) queuedLive(now time.Time) bool {
	return r.LeaseTTLSec > 0 && !r.leaseExpired(now)
}

func latest(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earliest(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func waiterReason(w Reservation, now time.Time) (string, WaiterInfo) {
	code := ReasonWaiterFirst
	if at, err := time.Parse(time.RFC3339, w.Created); err == nil && now.Sub(at) > waiterBound {
		code = ReasonWaiterBound
	}
	return code, WaiterInfo{Holder: w.Holder, Work: w.Work, QueuedAt: w.Created}
}
