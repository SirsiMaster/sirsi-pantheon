package schedule

// Multi-resource reserve (ledger task maat-multi-resource-reserve): `sirsi maat
// reserve r1 r2 ...` takes every resource or none. Each resource is admitted by
// the SAME procedure a single reserve uses (admit), in turn, against the rows
// the earlier resources just produced, and the whole result is written once: one
// transaction, one lock, one save. A refusal on any resource (a waiter that
// goes first, a refused delegated resource, an invalid request) refuses the
// whole request and writes nothing. Shares are per resource: one may be full
// and another a floor beside a live holder. No schema change; no --queue.

import (
	"errors"
	"fmt"
)

// ErrMultiUnsupported: the router service predates multi-resource reserve.
// Callers fall back to one reserve per resource (and own the partial failure).
var ErrMultiUnsupported = errors.New("maat: this router service does not support multi-resource reserve (not supported); reserve each resource separately")

// ReasonMultiUnsupported is the reason_code a caller reads to fall back.
const ReasonMultiUnsupported = "not-supported"

// maxMultiResources bounds one request; real use is two or three rails.
const maxMultiResources = 16

// MultiReservation is one granted resource of a multi-resource request.
// Reason, Cores and PendingCedes are additive: they say why a floor share was
// given, so a caller (and the decision journal) need not re-read the row.
type MultiReservation struct {
	Resource     string       `json:"resource"`
	ID           string       `json:"id"`
	Share        string       `json:"share"`
	Regime       Regime       `json:"regime"`
	Conflict     *Reservation `json:"conflict,omitempty"`
	Reason       string       `json:"reason,omitempty"`
	Cores        int          `json:"cores,omitempty"`
	PendingCedes []string     `json:"pending_cedes,omitempty"`
}

// MultiResult is the outcome of a multi-resource reserve. On a refusal Granted
// is false, Reservations is empty, and ReasonCode/Error name the resource and why.
type MultiResult struct {
	Granted      bool               `json:"granted"`
	Reservations []MultiReservation `json:"reservations"`
	ReasonCode   string             `json:"reason_code,omitempty"`
	Error        string             `json:"error,omitempty"`
}

// MultiRefusal renders a refusal error as the JSON-shaped result.
func MultiRefusal(err error) MultiResult {
	out := MultiResult{Reservations: []MultiReservation{}, Error: err.Error()}
	var wf *WaiterFirstError
	switch {
	case errors.As(err, &wf):
		out.ReasonCode = wf.ReasonCode
	case errors.Is(err, ErrMultiUnsupported):
		out.ReasonCode = ReasonMultiUnsupported
	}
	return out
}

// MultiScheduler is the optional native entry point for a multi-resource
// reserve (the router service). A store without it that is an AtomicScheduler
// is an older service: ReserveMulti then reports ErrMultiUnsupported rather
// than degrade into one write per resource.
type MultiScheduler interface {
	MaatReserveMulti([]Reservation) (MultiResult, error)
}

// ReserveMulti grants every request or none, in one write.
func (l *Ledger) ReserveMulti(reqs []Reservation) (MultiResult, error) {
	if m, ok := l.store.(MultiScheduler); ok {
		return m.MaatReserveMulti(reqs)
	}
	if _, ok := l.store.(AtomicScheduler); ok {
		return MultiResult{}, ErrMultiUnsupported
	}
	if len(reqs) == 0 || len(reqs) > maxMultiResources {
		return MultiResult{}, fmt.Errorf("maat: multi reserve takes 1..%d resources, got %d", maxMultiResources, len(reqs))
	}
	reqs = append([]Reservation(nil), reqs...)
	for i := range reqs {
		if err := validate(reqs[i]); err != nil {
			return MultiResult{}, err
		}
		for _, prev := range reqs[:i] {
			if sameResource(prev.Resource, reqs[i].Resource) {
				return MultiResult{}, fmt.Errorf("maat: resource %q listed twice", reqs[i].Resource)
			}
		}
		id, err := l.NewReservationID(reqs[i].Resource, reqs[i].Holder)
		if err != nil {
			return MultiResult{}, err
		}
		reqs[i].ID = id
	}
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		out, err := l.reserveMultiAttempt(reqs)
		if !errors.Is(err, errLedgerConflict) {
			return out, err
		}
	}
	return MultiResult{}, fmt.Errorf("maat: reserve: contention limit: %w", errLedgerConflict)
}

func (l *Ledger) reserveMultiAttempt(reqs []Reservation) (MultiResult, error) {
	raw, exists, rows, _, err := l.snapshot()
	if err != nil {
		return MultiResult{}, err
	}
	out := MultiResult{Granted: true, Reservations: make([]MultiReservation, 0, len(reqs))}
	for _, req := range reqs {
		var res ReserveResult
		rows, _, res, err = l.admit(req, false, false, rows)
		if err != nil {
			var wf *WaiterFirstError
			if errors.As(err, &wf) {
				return MultiResult{}, err // names the resource, the waiter and the reason
			}
			return MultiResult{}, fmt.Errorf("maat: reserve %s refused (nothing reserved): %w", req.Resource, err)
		}
		r := res.Reservation
		out.Reservations = append(out.Reservations, MultiReservation{
			Resource: r.Resource, ID: r.ID, Share: r.Share, Regime: r.Regime, Conflict: res.Conflict,
			Reason: r.Reason, Cores: r.Cores, PendingCedes: r.PendingCedes,
		})
	}
	if err := l.save(raw, exists, rows); err != nil {
		return MultiResult{}, err
	}
	return out, nil
}
