// Willing cessation (owner directive, 2026-09-26): Ma'at never forces a lane
// off a machine and never stops a run mid-run. Priority decides who wins a
// conflicting NEW reservation; it never preempts a live one. A lane that
// needs time, cores, or a whole machine files a Cede request against a
// resource and its current holder. The holder alone answers — grant (with a
// start time, e.g. "after my current run"), counter (less, shorter, later),
// or decline — and an unanswered request stays pending forever; nothing here
// auto-grants it. A grant does not move any reservation: the holder finishes
// its run and releases/does not renew, and the requester then reserves
// normally through the existing ledger. Every ask is bounded (<=60 minutes)
// so every lane keeps getting time on every machine, and the whole exchange
// (who asked, who ceded, what, when, why) is recorded for audit.
package schedule

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// cedeStateKey is the ledger for cede requests — kept separate from
// "maat:reservations" because a cede is a negotiation, not a hold: it never
// changes any reservation's status by itself (see cede_test.go).
const cedeStateKey = "maat:cedes"

// CedeStatus is a cede request's lifecycle state.
type CedeStatus string

const (
	CedeStatusPending   CedeStatus = "pending"
	CedeStatusGranted   CedeStatus = "granted"
	CedeStatusCountered CedeStatus = "countered"
	CedeStatusDeclined  CedeStatus = "declined"
	CedeStatusWithdrawn CedeStatus = "withdrawn"
	CedeStatusDone      CedeStatus = "done"
)

// CedeDecision is the holder's willing answer to a Cede request. Exactly one
// of grant/counter/decline — recorded with who decided and why, so the
// exchange is auditable even though it never touches the reservation ledger.
type CedeDecision struct {
	By      string `json:"by"` // agent id of the holder who answered
	Reason  string `json:"reason"`
	StartAt string `json:"start_at,omitempty"` // grant: RFC3339, or "after my current run"
	Counter string `json:"counter,omitempty"`  // counter: the counter-offer, e.g. "cores:2 for 20m at 15:00Z"
	At      string `json:"at"`                 // RFC3339 — when the decision was made
}

// Cede is one request from Requester asking Holder to give up time, cores, or
// a whole machine on Resource.
type Cede struct {
	ID            string        `json:"id"`
	Resource      string        `json:"resource"`       // machine id or rail, e.g. "m1", "rail-a"
	Requester     string        `json:"requester"`      // agent id asking
	Holder        string        `json:"holder"`         // agent id who currently holds it and must answer
	Ask           string        `json:"ask"`            // "machine" or "cores:N" (N 1..64)
	Minutes       int           `json:"minutes"`        // bounded 1..60
	EarliestStart string        `json:"earliest_start"` // RFC3339
	Reason        string        `json:"reason"`
	Status        CedeStatus    `json:"status"`
	Decision      *CedeDecision `json:"decision,omitempty"`
	Created       string        `json:"created"` // RFC3339
}

func cedeIsTerminal(s CedeStatus) bool {
	switch s {
	case CedeStatusDeclined, CedeStatusWithdrawn, CedeStatusDone:
		return true
	}
	return false
}

// loadCedes reads the cede ledger (no expiry sweep — a cede has no lease; it
// stays pending until the holder answers or the requester withdraws).
func (l *Ledger) loadCedes() ([]Cede, error) {
	raw, ok, err := l.store.GetState(cedeStateKey)
	if err != nil {
		return nil, fmt.Errorf("maat: read cede ledger: %w", err)
	}
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var doc struct {
		Cedes []Cede `json:"cedes"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("maat: parse cede ledger: %w", err)
	}
	return doc.Cedes, nil
}

// saveCedes persists the cede ledger, dropping terminal rows older than 24h
// (same retention policy as the reservation ledger).
func (l *Ledger) saveCedes(cedes []Cede) error {
	cutoff := l.now().Add(-24 * time.Hour)
	kept := cedes[:0]
	for _, c := range cedes {
		if cedeIsTerminal(c.Status) {
			if created, err := time.Parse(time.RFC3339, c.Created); err == nil && created.Before(cutoff) {
				continue
			}
		}
		kept = append(kept, c)
	}
	doc := struct {
		Cedes []Cede `json:"cedes"`
	}{Cedes: kept}
	b, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("maat: marshal cede ledger: %w", err)
	}
	return l.store.SetState(cedeStateKey, string(b))
}

// validateAsk accepts exactly "machine" or "cores:N" with N in 1..64.
func validateAsk(ask string) error {
	if ask == "machine" {
		return nil
	}
	n, ok := strings.CutPrefix(ask, "cores:")
	if !ok {
		return fmt.Errorf("maat: ask %q invalid (want \"machine\" or \"cores:N\")", ask)
	}
	v, err := strconv.Atoi(n)
	if err != nil || v < 1 || v > 64 {
		return fmt.Errorf("maat: ask %q invalid (cores:N wants 1<=N<=64)", ask)
	}
	return nil
}

// RequestCede files a cede request. It never touches the reservation ledger —
// filing an ask is not a hold, and never changes any reservation's status.
func (l *Ledger) RequestCede(req Cede) (*Cede, error) {
	if !resourceRe.MatchString(req.Resource) {
		return nil, fmt.Errorf("maat: resource %q invalid (want a lowercase slug like m1, rail-a)", req.Resource)
	}
	if strings.TrimSpace(req.Requester) == "" {
		return nil, fmt.Errorf("maat: requester (agent id) required")
	}
	if strings.TrimSpace(req.Holder) == "" {
		return nil, fmt.Errorf("maat: holder (agent id) required")
	}
	if err := validateAsk(req.Ask); err != nil {
		return nil, err
	}
	if req.Minutes < 1 || req.Minutes > 60 {
		return nil, fmt.Errorf("maat: minutes %d invalid (cedes are bounded 1..60)", req.Minutes)
	}
	if req.EarliestStart != "" {
		if _, err := time.Parse(time.RFC3339, req.EarliestStart); err != nil {
			return nil, fmt.Errorf("maat: earliest_start %q is not RFC3339", req.EarliestStart)
		}
	}
	if strings.TrimSpace(req.Reason) == "" {
		return nil, fmt.Errorf("maat: reason required")
	}

	cedes, err := l.loadCedes()
	if err != nil {
		return nil, err
	}
	now := l.now()
	req.Created = now.Format(time.RFC3339)
	req.Status = CedeStatusPending
	req.Decision = nil
	if req.ID == "" {
		req.ID = newCedeID(req.Resource, req.Requester, now)
	}
	cedes = append(cedes, req)
	if err := l.saveCedes(cedes); err != nil {
		return nil, err
	}
	out := req
	return &out, nil
}

// RespondCede is the holder's willing answer: grant, counter, or decline.
// Only the named holder may answer, and only while the request is pending —
// an unanswered request otherwise stays pending forever (no auto-grant).
// A grant/counter/decline never itself moves any reservation.
func (l *Ledger) RespondCede(id, by string, status CedeStatus, reason, startAt, counter string) (*Cede, error) {
	switch status {
	case CedeStatusGranted, CedeStatusCountered, CedeStatusDeclined:
	default:
		return nil, fmt.Errorf("maat: respond status %q invalid (want granted|countered|declined)", status)
	}
	cedes, err := l.loadCedes()
	if err != nil {
		return nil, err
	}
	for i := range cedes {
		if cedes[i].ID != id {
			continue
		}
		c := &cedes[i]
		if c.Status != CedeStatusPending {
			return nil, fmt.Errorf("maat: cede %s is %s, not pending", id, c.Status)
		}
		if c.Holder != by {
			return nil, fmt.Errorf("maat: cede %s must be answered by holder %s, not %s", id, c.Holder, by)
		}
		c.Status = status
		c.Decision = &CedeDecision{By: by, Reason: reason, StartAt: startAt, Counter: counter, At: l.now().Format(time.RFC3339)}
		if err := l.saveCedes(cedes); err != nil {
			return nil, err
		}
		out := *c
		return &out, nil
	}
	return nil, fmt.Errorf("maat: cede %s not found", id)
}

// WithdrawCede lets the requester pull back its own still-pending ask.
func (l *Ledger) WithdrawCede(id, by string) (*Cede, error) {
	cedes, err := l.loadCedes()
	if err != nil {
		return nil, err
	}
	for i := range cedes {
		if cedes[i].ID != id {
			continue
		}
		c := &cedes[i]
		if c.Requester != by {
			return nil, fmt.Errorf("maat: cede %s must be withdrawn by requester %s, not %s", id, c.Requester, by)
		}
		if c.Status != CedeStatusPending {
			return nil, fmt.Errorf("maat: cede %s is %s, not pending", id, c.Status)
		}
		c.Status = CedeStatusWithdrawn
		if err := l.saveCedes(cedes); err != nil {
			return nil, err
		}
		out := *c
		return &out, nil
	}
	return nil, fmt.Errorf("maat: cede %s not found", id)
}

// CedeFilter narrows ListCedes.
type CedeFilter struct {
	Resource    string
	Holder      string
	Requester   string
	PendingOnly bool
}

// ListCedes returns cedes matching filter, newest first.
func (l *Ledger) ListCedes(filter CedeFilter) ([]Cede, error) {
	cedes, err := l.loadCedes()
	if err != nil {
		return nil, err
	}
	out := make([]Cede, 0, len(cedes))
	for _, c := range cedes {
		if filter.Resource != "" && c.Resource != filter.Resource {
			continue
		}
		if filter.Holder != "" && c.Holder != filter.Holder {
			continue
		}
		if filter.Requester != "" && c.Requester != filter.Requester {
			continue
		}
		if filter.PendingOnly && c.Status != CedeStatusPending {
			continue
		}
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created > out[j].Created })
	return out, nil
}

func newCedeID(resource, requester string, now time.Time) string {
	slug := strings.NewReplacer("/", "-", " ", "-", "@", "-").Replace(requester)
	return fmt.Sprintf("cede-%s-%s-%s", now.UTC().Format("20060102T150405Z"), resource, slug)
}
