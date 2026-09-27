// Package schedule implements the Maat metal reservation scheduler
// (stacklab.wing.maat, MAAT-WING-001.G1): a per-machine and per-resource
// reservation ledger with heartbeat leases and enforced admission, so lanes
// sharing the same Macs and Thunderbolt rails cannot contaminate each other's
// measurement windows.
//
// The ledger is stored in the shared router store (via GetState/SetState), so a
// reservation made on one Mac is visible to every other Mac — an M1 reservation
// stops an M5 runner. Resources are FREE-FORM strings, never a hardcoded machine
// list: any present-or-future Mac participates by reserving its own
// machine-scoped resources with no code change (owner directive 2026-09-24).
//
// Design ceilings (deliberate, named):
//   - Grant is a read-modify-write on one state key. Two hosts reserving the
//     SAME resource within the few-millisecond RMW window could both win. The
//     reservation cadence is a handful per hour across lanes, so the collision
//     probability is negligible; upgrade to a compare-and-swap on the state key
//     if throughput ever demands it. (maat: RMW grant, CAS if contended.)
//
// Ma'at never preempts and never locks a lane out (owner directive
// 2026-09-26): every lane always gets at least its floor share; priority and
// cedes decide who gets more. A conflicting NEW reservation is granted a
// bounded floor share (capacity.go) rather than refused; nothing pulls a
// live holder off its window. A lane that has not answered a cede request
// addressed to it (cede.go) is likewise never locked out — it is capped to
// the floor share for any new or extended work until it answers.
package schedule

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// stateKey is the single router-store key holding the whole reservation ledger
// as one JSON document. One document keeps the cross-host read cheap and the
// overlap check in-process; the ledger stays small (active + recently-closed).
const stateKey = "maat:reservations"

// Regime is the load regime a reservation declares for its window.
type Regime string

const (
	RegimeQuiet  Regime = "quiet"  // measurement — no foreign load tolerated
	RegimeLoaded Regime = "loaded" // load test — the holder's own load is expected
	RegimeBuild  Regime = "build"  // a build/CI job — lowest measurement sensitivity
)

// Status is a reservation's lifecycle state.
type Status string

const (
	StatusActive      Status = "active"
	StatusQueued      Status = "queued"
	StatusReleased    Status = "released"
	StatusExpired     Status = "expired"     // heartbeat lease lapsed — no stale locks
	StatusInvalidated Status = "invalidated" // a foreign intruder contaminated the window
	StatusDone        Status = "done"        // window ended cleanly
)

// resourceRe validates a resource id: a lowercase slug, so any machine id or
// rail name is accepted but shell/JSON-hostile input is not. NOT an allowlist —
// new Macs and new rails work with no code change.
var resourceRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._@-]{0,63}$`)

// Reservation is one hold on one resource for one time window.
type Reservation struct {
	ID       string `json:"id"`
	Resource string `json:"resource"` // machine id or rail, e.g. "m1", "rail-a", "ci-runners@m5"
	Holder   string `json:"holder"`   // agent id
	Work     string `json:"work"`     // series/arm, e.g. "H6.2/sr-A-fwd"
	Regime   Regime `json:"regime"`
	Priority int    `json:"priority"`
	// Cores is an ADVISORY core count: Ma'at itself never limits CPU. 0 means
	// "the whole machine" (the pre-fair-share default). Callers enforce it
	// themselves — GOMAXPROCS, a worker-pool cap, `nice` — same as Regime is
	// advisory. Share says whether Cores is the full ask ("full") or a
	// bounded floor share granted alongside a live conflict or an unanswered
	// cede request ("floor"); Reason explains which (owner directive
	// 2026-09-26: "never totally block lanes from operation").
	Cores int    `json:"cores,omitempty"`
	Share string `json:"share,omitempty"` // "full" | "floor"
	// MemGB is an ADVISORY memory ask in GB — same posture as Cores, Ma'at
	// never enforces RSS itself. Unlike Cores, 0 means "no explicit ask", NOT
	// "the whole machine": an unbounded ask is the exact failure mode the
	// 2026-09-26 M1 stall exposed (no gate read memory at all). MemShare
	// mirrors Share but is set independently — see plan.go/conflict.go
	// pressure wiring.
	MemGB         int      `json:"mem_gb,omitempty"`
	MemShare      string   `json:"mem_share,omitempty"` // "full" | "floor"
	Reason        string   `json:"reason,omitempty"`
	PendingCedes  []string `json:"pending_cedes,omitempty"` // open cede ids capping this ticket to the floor
	Repro         string   `json:"repro,omitempty"`         // repro path
	Start         string   `json:"start"`                   // RFC3339
	EstEnd        string   `json:"est_end"`                 // RFC3339
	Created       string   `json:"created"`                 // RFC3339
	HeartbeatAt   string   `json:"heartbeat_at"`            // RFC3339 — lease liveness
	LeaseTTLSec   int      `json:"lease_ttl_sec"`           // reservation void if heartbeat older than this
	Status        Status   `json:"status"`
	InvalidatedBy string   `json:"invalidated_by,omitempty"` // intruder description
}

// ShareFull and ShareFloor are the two values Reservation.Share takes.
const (
	ShareFull  = "full"
	ShareFloor = "floor"
)

// leaseExpired reports whether the heartbeat lease has lapsed as of now.
func (r Reservation) leaseExpired(now time.Time) bool {
	if r.LeaseTTLSec <= 0 {
		return false
	}
	hb, err := time.Parse(time.RFC3339, r.HeartbeatAt)
	if err != nil {
		return true // an unparseable heartbeat is a dead lease, never a live one
	}
	return now.After(hb.Add(time.Duration(r.LeaseTTLSec) * time.Second))
}

// windowOverlaps reports whether this reservation's [Start,EstEnd] overlaps other's.
func (r Reservation) windowOverlaps(o Reservation) bool {
	rs, re := parseWindow(r)
	os, oe := parseWindow(o)
	// half-open overlap: two windows overlap unless one ends at/before the other starts.
	return rs.Before(oe) && os.Before(re)
}

func parseWindow(r Reservation) (start, end time.Time) {
	start, err := time.Parse(time.RFC3339, r.Start)
	if err != nil {
		start = time.Time{}
	}
	end, err = time.Parse(time.RFC3339, r.EstEnd)
	if err != nil {
		// no/!bad est-end → treat as open-ended from start (a lease still bounds it)
		end = start.Add(24 * time.Hour)
	}
	return start, end
}

// StateStore is the minimal slice of the router store the ledger needs. The real
// routerstore.Store satisfies it; tests use an in-memory fake. Keeping it tiny
// avoids coupling the scheduler to the 90-method Store interface.
type StateStore interface {
	GetState(key string) (string, bool, error)
	SetState(key, value string) error
}

// Ledger is the reservation scheduler over a StateStore.
type Ledger struct {
	store StateStore
	now   func() time.Time // injectable clock (A16)
}

// NewLedger returns a Ledger backed by store, using the real clock.
func NewLedger(store StateStore) *Ledger {
	return &Ledger{store: store, now: time.Now}
}

// WithClock overrides the clock (tests).
func (l *Ledger) WithClock(now func() time.Time) *Ledger { l.now = now; return l }

// load reads and expiry-sweeps the ledger. Expiry runs on every read so a
// crashed holder's lease self-clears — no stale locks (a hard requirement).
// Returns the live reservations and whether the sweep changed anything.
func (l *Ledger) load() (reservations []Reservation, changed bool, err error) {
	raw, ok, err := l.store.GetState(stateKey)
	if err != nil {
		return nil, false, fmt.Errorf("maat: read ledger: %w", err)
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	var doc struct {
		Reservations []Reservation `json:"reservations"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, false, fmt.Errorf("maat: parse ledger: %w", err)
	}
	now := l.now()
	for i := range doc.Reservations {
		if doc.Reservations[i].Status == StatusActive && doc.Reservations[i].leaseExpired(now) {
			doc.Reservations[i].Status = StatusExpired
			changed = true
		}
	}
	return doc.Reservations, changed, nil
}

// save persists the ledger, dropping terminal reservations older than 24h so it
// stays bounded (recent history is kept for the dashboard/utilization view).
func (l *Ledger) save(reservations []Reservation) error {
	cutoff := l.now().Add(-24 * time.Hour)
	kept := reservations[:0]
	for _, r := range reservations {
		if isTerminal(r.Status) {
			if end, err := time.Parse(time.RFC3339, r.EstEnd); err == nil && end.Before(cutoff) {
				continue // prune old terminal rows
			}
		}
		kept = append(kept, r)
	}
	doc := struct {
		Reservations []Reservation `json:"reservations"`
	}{Reservations: kept}
	b, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("maat: marshal ledger: %w", err)
	}
	if err := l.store.SetState(stateKey, string(b)); err != nil {
		return fmt.Errorf("maat: write ledger: %w", err)
	}
	return nil
}

func isTerminal(s Status) bool {
	switch s {
	case StatusReleased, StatusExpired, StatusInvalidated, StatusDone:
		return true
	}
	return false
}

// ReserveResult is the outcome of a Reserve call. Granted is false only for
// invalid input (a validation error, returned separately) or an explicit
// --queue wait; every other request grants at least the floor share (owner
// directive 2026-09-26: "not totally block lanes from operation").
type ReserveResult struct {
	Granted      bool         `json:"granted"`
	Reservation  *Reservation `json:"reservation,omitempty"`
	Conflict     *Reservation `json:"conflict,omitempty"` // the live holder that caused a floor grant (or a queue wait)
	Queued       bool         `json:"queued,omitempty"`
	PendingCedes []string     `json:"pending_cedes,omitempty"` // open cede ids that capped this grant to the floor
}

// Reserve attempts to hold resource for the given window.
//
// Ma'at never locks a lane out (owner directive 2026-09-26: "not totally
// block lanes from operation... there's room for all of you to never be
// locked out"):
//   - No conflict, no pending cede: grants exactly what was asked (Share
//     "full"), as before.
//   - A live (active, non-expired) reservation by a DIFFERENT holder overlaps
//     the window: grants a FLOOR share instead of refusing (FloorShare,
//     capacity.go) — Cores set to the floor, Share "floor", Conflict names
//     the live holder, Reason explains it. --queue keeps its old meaning: it
//     queues for the FULL window behind the holder instead of taking a floor
//     grant now.
//   - req.Holder has any unanswered cede request addressed to it, on any
//     resource — including its own overlap-refresh path, since starting a
//     new window is new work: grants only the floor share, PendingCedes
//     names the open ids, Reason points at answering them. Heartbeat and
//     Release are unaffected: the holder's CURRENT run always finishes, and
//     any response or a withdraw restores full grants on the next Reserve.
func (l *Ledger) Reserve(req Reservation, queue bool) (ReserveResult, error) {
	if err := validate(req); err != nil {
		return ReserveResult{}, err
	}
	pendingCedes, err := l.pendingCedeIDs(req.Holder)
	if err != nil {
		return ReserveResult{}, err
	}
	floor, err := l.FloorShare(req.Resource)
	if err != nil {
		return ReserveResult{}, err
	}
	reservations, _, err := l.load()
	if err != nil {
		return ReserveResult{}, err
	}
	now := l.now()
	req.Created = now.Format(time.RFC3339)
	req.HeartbeatAt = req.Created
	if req.LeaseTTLSec == 0 {
		req.LeaseTTLSec = 120
	}
	if req.ID == "" {
		req.ID = newID(req.Resource, req.Holder, now)
	}

	if len(pendingCedes) > 0 {
		req.Cores = floor
		req.Share = ShareFloor
		req.Reason = "answer open cede requests to get more than the floor"
		req.PendingCedes = pendingCedes
		req.Status = StatusActive
		reservations = append(reservations, req)
		if err := l.save(reservations); err != nil {
			return ReserveResult{}, err
		}
		return ReserveResult{Granted: true, Reservation: &req, PendingCedes: pendingCedes}, nil
	}

	for _, r := range reservations {
		if r.Resource != req.Resource || r.Status != StatusActive {
			continue
		}
		if r.leaseExpired(now) {
			continue
		}
		if !r.windowOverlaps(req) {
			continue
		}
		if r.Holder == req.Holder {
			continue // own overlapping hold — the new one is additive/refresh, allow
		}
		// A live foreign holder is on the window — --queue waits for the
		// full window; otherwise grant the floor share now (never a lockout).
		if queue {
			req.Status = StatusQueued
			reservations = append(reservations, req)
			if err := l.save(reservations); err != nil {
				return ReserveResult{}, err
			}
			conflict := r
			return ReserveResult{Granted: false, Queued: true, Reservation: &req, Conflict: &conflict}, nil
		}
		req.Cores = floor
		req.Share = ShareFloor
		req.Reason = fmt.Sprintf("%s holds %s until %s — granted the floor share", r.Holder, req.Resource, orOpen(r.EstEnd))
		req.Status = StatusActive
		reservations = append(reservations, req)
		if err := l.save(reservations); err != nil {
			return ReserveResult{}, err
		}
		conflict := r
		return ReserveResult{Granted: true, Reservation: &req, Conflict: &conflict}, nil
	}

	req.Share = ShareFull
	req.Status = StatusActive
	reservations = append(reservations, req)
	if err := l.save(reservations); err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{Granted: true, Reservation: &req}, nil
}

// orOpen renders an est-end for a Reason string ("open" if unset).
func orOpen(estEnd string) string {
	if estEnd == "" {
		return "open"
	}
	return estEnd
}

// Heartbeat refreshes a reservation's lease so it does not expire. Returns the
// updated reservation. A released/expired reservation cannot be revived.
func (l *Ledger) Heartbeat(id string) (*Reservation, error) {
	return l.mutate(id, func(r *Reservation) error {
		if r.Status != StatusActive {
			return fmt.Errorf("maat: reservation %s is %s, not active", id, r.Status)
		}
		r.HeartbeatAt = l.now().Format(time.RFC3339)
		return nil
	})
}

// Extend pushes the est-end out (and refreshes the lease). Never refused over
// an unanswered cede (owner directive 2026-09-26: no lockout) — the ticket
// keeps running; if req.Holder has a pending cede, its cores are capped to
// the floor share (PendingCedes recorded) instead of refusing the extend.
// Heartbeat and Release are unaffected.
func (l *Ledger) Extend(id, newEstEnd string) (*Reservation, error) {
	if _, err := time.Parse(time.RFC3339, newEstEnd); err != nil {
		return nil, fmt.Errorf("maat: extend: est_end %q is not RFC3339", newEstEnd)
	}
	return l.mutate(id, func(r *Reservation) error {
		if r.Status != StatusActive {
			return fmt.Errorf("maat: reservation %s is %s, not active", id, r.Status)
		}
		pendingCedes, err := l.pendingCedeIDs(r.Holder)
		if err != nil {
			return err
		}
		if len(pendingCedes) > 0 {
			floor, ferr := l.FloorShare(r.Resource)
			if ferr != nil {
				return ferr
			}
			if r.Cores == 0 || r.Cores > floor {
				r.Cores = floor
			}
			r.Share = ShareFloor
			r.Reason = "answer open cede requests to get more than the floor"
			r.PendingCedes = pendingCedes
		} else {
			r.PendingCedes = nil
		}
		r.EstEnd = newEstEnd
		r.HeartbeatAt = l.now().Format(time.RFC3339)
		return nil
	})
}

// Release ends a reservation cleanly, freeing the resource immediately.
func (l *Ledger) Release(id string) (*Reservation, error) {
	return l.mutate(id, func(r *Reservation) error {
		if isTerminal(r.Status) {
			return fmt.Errorf("maat: reservation %s already %s", id, r.Status)
		}
		r.Status = StatusReleased
		return nil
	})
}

// Invalidate marks a reservation's block contaminated by a named intruder
// (conflict detection, P2). The window is recorded INVALID so the measurement is
// not trusted.
func (l *Ledger) Invalidate(id, intruder string) (*Reservation, error) {
	return l.mutate(id, func(r *Reservation) error {
		r.Status = StatusInvalidated
		r.InvalidatedBy = intruder
		return nil
	})
}

func (l *Ledger) mutate(id string, fn func(*Reservation) error) (*Reservation, error) {
	reservations, _, err := l.load()
	if err != nil {
		return nil, err
	}
	for i := range reservations {
		if reservations[i].ID == id {
			if err := fn(&reservations[i]); err != nil {
				return nil, err
			}
			if err := l.save(reservations); err != nil {
				return nil, err
			}
			out := reservations[i]
			return &out, nil
		}
	}
	return nil, fmt.Errorf("maat: reservation %s not found", id)
}

// WhoIsOn returns the current live (active, non-expired) holder of a resource
// whose window covers now, or nil if the resource is free.
func (l *Ledger) WhoIsOn(resource string) (*Reservation, error) {
	reservations, changed, err := l.load()
	if err != nil {
		return nil, err
	}
	if changed {
		_ = l.save(reservations)
	}
	now := l.now()
	for _, r := range reservations {
		if r.Resource != resource || r.Status != StatusActive || r.leaseExpired(now) {
			continue
		}
		s, e := parseWindow(r)
		if !now.Before(s) && now.Before(e) {
			rr := r
			return &rr, nil
		}
	}
	return nil, nil
}

// LiveHolders returns every active, non-expired reservation on resource whose
// window covers now — the full-share holder (if any) plus any floor-share
// lanes riding beside it (owner directive 2026-09-26: Ma'at never locks a
// lane out, so more than one holder can be live on the same resource at
// once). Empty when the resource is free.
func (l *Ledger) LiveHolders(resource string) ([]Reservation, error) {
	reservations, changed, err := l.load()
	if err != nil {
		return nil, err
	}
	if changed {
		_ = l.save(reservations)
	}
	now := l.now()
	var out []Reservation
	for _, r := range reservations {
		if r.Resource != resource || r.Status != StatusActive || r.leaseExpired(now) {
			continue
		}
		s, e := parseWindow(r)
		if !now.Before(s) && now.Before(e) {
			out = append(out, r)
		}
	}
	return out, nil
}

// Status lists reservations, optionally filtered to one resource, newest window
// first. Terminal rows within the retention window are included so a caller can
// show recent history and per-day conflict counts.
func (l *Ledger) Status(resource string) ([]Reservation, error) {
	reservations, changed, err := l.load()
	if err != nil {
		return nil, err
	}
	if changed {
		_ = l.save(reservations)
	}
	out := make([]Reservation, 0, len(reservations))
	for _, r := range reservations {
		if resource == "" || r.Resource == resource {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start > out[j].Start })
	return out, nil
}

func validate(r Reservation) error {
	if !resourceRe.MatchString(r.Resource) {
		return fmt.Errorf("maat: resource %q invalid (want a lowercase slug like m1, rail-a, ci-runners@m5)", r.Resource)
	}
	if strings.TrimSpace(r.Holder) == "" {
		return fmt.Errorf("maat: holder (agent id) required")
	}
	switch r.Regime {
	case RegimeQuiet, RegimeLoaded, RegimeBuild:
	case "":
		return fmt.Errorf("maat: regime required (quiet|loaded|build)")
	default:
		return fmt.Errorf("maat: regime %q invalid (want quiet|loaded|build)", r.Regime)
	}
	if _, err := time.Parse(time.RFC3339, r.Start); err != nil {
		return fmt.Errorf("maat: start %q is not RFC3339", r.Start)
	}
	if r.EstEnd != "" {
		if _, err := time.Parse(time.RFC3339, r.EstEnd); err != nil {
			return fmt.Errorf("maat: est_end %q is not RFC3339", r.EstEnd)
		}
	}
	return nil
}

func newID(resource, holder string, now time.Time) string {
	slug := strings.NewReplacer("/", "-", " ", "-", "@", "-").Replace(holder)
	return fmt.Sprintf("%s-%s-%s", now.UTC().Format("20060102T150405Z"), resource, slug)
}
