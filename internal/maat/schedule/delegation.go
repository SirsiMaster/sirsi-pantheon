// ADR-120 step 1: the pod-local Ma'at booking authority's delegation record,
// cloud-side fence, and the idempotent upsert reconciliation replays onto.
// Design-only until a pod authority process (step 2) actually calls
// ReserveAsAuthority/UpsertReservation; the fence in Reserve below is live
// today for any caller, since it is the same admission chokepoint every
// Reserve call (local or server-transactional) already goes through.
package schedule

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

const delegationStateKey = "maat:delegations"

// maxDelegationTTL is the finite window cap (ADR-120 §4): a delegation never
// renews itself past this without the canonical ledger being reachable.
const maxDelegationTTL = 12 * time.Hour

var podRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

// Delegation is a finite, fenced grant of exactly Resources to one pod
// authority (ADR-120 §1). Minted by the canonical ledger; the pod authority
// never manufactures or extends one on its own.
type Delegation struct {
	Pod              string   `json:"pod"`
	AuthorityMachine string   `json:"authority_machine"`
	AuthorityAddrs   []string `json:"authority_addrs,omitempty"`
	Resources        []string `json:"resources"`
	Epoch            int      `json:"epoch"`
	IssuedAt         string   `json:"issued_at"` // RFC3339
	NotAfter         string   `json:"not_after"` // RFC3339, at most IssuedAt+12h
	AckedSeq         int64    `json:"acked_seq,omitempty"`
	PodSecretHash    string   `json:"pod_secret_hash,omitempty"`
	Revoked          bool     `json:"revoked,omitempty"`
}

func (d Delegation) expired(now time.Time) bool {
	if d.Revoked {
		return true
	}
	t, err := time.Parse(time.RFC3339, d.NotAfter)
	if err != nil {
		return true // an unparseable not_after is a dead delegation, never a live one
	}
	return !now.Before(t)
}

func (d Delegation) coversResource(resource string) bool {
	for _, r := range d.Resources {
		if sameResource(r, resource) {
			return true
		}
	}
	return false
}

func validateDelegation(d Delegation) error {
	if !podRe.MatchString(d.Pod) {
		return fmt.Errorf("maat: pod %q invalid (want a lowercase slug like studio-m5)", d.Pod)
	}
	if strings.TrimSpace(d.AuthorityMachine) == "" {
		return fmt.Errorf("maat: authority_machine required")
	}
	if len(d.Resources) == 0 {
		return fmt.Errorf("maat: at least one resource required")
	}
	for _, r := range d.Resources {
		if !resourceRe.MatchString(r) {
			return fmt.Errorf("maat: resource %q invalid", r)
		}
	}
	issued, err := time.Parse(time.RFC3339, d.IssuedAt)
	if err != nil {
		return fmt.Errorf("maat: issued_at %q is not RFC3339", d.IssuedAt)
	}
	notAfter, err := time.Parse(time.RFC3339, d.NotAfter)
	if err != nil {
		return fmt.Errorf("maat: not_after %q is not RFC3339", d.NotAfter)
	}
	if notAfter.Sub(issued) > maxDelegationTTL {
		return fmt.Errorf("maat: delegation window %s exceeds the %s finite cap", notAfter.Sub(issued), maxDelegationTTL)
	}
	return nil
}

func (l *Ledger) delegationSnapshot() (raw string, exists bool, rows []Delegation, err error) {
	raw, exists, err = l.store.GetState(delegationStateKey)
	if err != nil {
		return "", false, nil, fmt.Errorf("maat: read delegation ledger: %w", err)
	}
	if !exists || strings.TrimSpace(raw) == "" {
		return raw, exists, nil, nil
	}
	var doc struct {
		Delegations []Delegation `json:"delegations"`
	}
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return "", false, nil, fmt.Errorf("maat: parse delegation ledger: %w", err)
	}
	return raw, exists, doc.Delegations, nil
}

func (l *Ledger) delegationSave(raw string, exists bool, rows []Delegation) error {
	store, ok := l.store.(atomicStateStore)
	if !ok {
		return fmt.Errorf("maat: store lacks atomic delegation writes; upgrade server/client")
	}
	encoded, err := json.Marshal(rows)
	if err != nil {
		return fmt.Errorf("maat: marshal delegations: %w", err)
	}
	out, err := json.Marshal(map[string]json.RawMessage{"delegations": encoded})
	if err != nil {
		return err
	}
	swapped, err := store.CompareAndSwapState(delegationStateKey, raw, exists, string(out))
	if err != nil {
		return fmt.Errorf("maat: write delegation ledger: %w", err)
	}
	if !swapped {
		return errLedgerConflict
	}
	return nil
}

// Delegate creates, or re-issues, the delegation for req.Pod. Re-issuing the
// SAME pod bumps its own epoch (how authority moves to a new
// authority_machine) rather than refusing; a resource already covered by a
// live (non-expired, non-revoked) delegation for a DIFFERENT pod is refused
// (ADR-120 §1: a resource belongs to at most one live delegation).
func (l *Ledger) Delegate(req Delegation) (Delegation, error) {
	if err := validateDelegation(req); err != nil {
		return Delegation{}, err
	}
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		out, err := l.delegateAttempt(req)
		if !errors.Is(err, errLedgerConflict) {
			return out, err
		}
	}
	return Delegation{}, fmt.Errorf("maat: delegate: contention limit: %w", errLedgerConflict)
}

func (l *Ledger) delegateAttempt(req Delegation) (Delegation, error) {
	raw, exists, rows, err := l.delegationSnapshot()
	if err != nil {
		return Delegation{}, err
	}
	now := l.now()
	nextEpoch := 1
	replaceAt := -1
	for i, d := range rows {
		if d.Pod == req.Pod {
			replaceAt = i
			nextEpoch = d.Epoch + 1
			continue
		}
		if d.expired(now) {
			continue
		}
		for _, r := range req.Resources {
			if d.coversResource(r) {
				return Delegation{}, fmt.Errorf("maat: resource %q already delegated to pod %q (epoch %d)", r, d.Pod, d.Epoch)
			}
		}
	}
	req.Epoch = nextEpoch
	req.Revoked = false
	if replaceAt >= 0 {
		rows[replaceAt] = req
	} else {
		rows = append(rows, req)
	}
	if err := l.delegationSave(raw, exists, rows); err != nil {
		return Delegation{}, err
	}
	return req, nil
}

// Revoke ends pod's delegation immediately. The fence (ReserveAsAuthority)
// stops granting at once; Reserve starts admitting the resources directly
// again as soon as expired() sees Revoked.
func (l *Ledger) Revoke(pod string) (Delegation, error) {
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		out, err := l.revokeAttempt(pod)
		if !errors.Is(err, errLedgerConflict) {
			return out, err
		}
	}
	return Delegation{}, fmt.Errorf("maat: revoke: contention limit: %w", errLedgerConflict)
}

func (l *Ledger) revokeAttempt(pod string) (Delegation, error) {
	raw, exists, rows, err := l.delegationSnapshot()
	if err != nil {
		return Delegation{}, err
	}
	for i, d := range rows {
		if d.Pod == pod {
			rows[i].Revoked = true
			if err := l.delegationSave(raw, exists, rows); err != nil {
				return Delegation{}, err
			}
			return rows[i], nil
		}
	}
	return Delegation{}, fmt.Errorf("maat: no delegation for pod %q", pod)
}

// RenewDelegation extends pod's current delegation window to now+12h
// (ADR-120 §4), without bumping Epoch -- a distinct operation from Delegate's
// reissue, which always bumps epoch (that is for moving authority to a new
// machine, not a routine renewal). Refuses a revoked delegation, or one whose
// Epoch no longer matches the caller's: both ARE the self-fence check step 2's
// pod authority relies on (ADR-120 §3) -- a superseded or revoked instance
// renewing must see that refusal, not silently keep going.
func (l *Ledger) RenewDelegation(pod string, epoch int, now time.Time) (Delegation, error) {
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		out, err := l.renewDelegationAttempt(pod, epoch, now)
		if !errors.Is(err, errLedgerConflict) {
			return out, err
		}
	}
	return Delegation{}, fmt.Errorf("maat: renew delegation: contention limit: %w", errLedgerConflict)
}

func (l *Ledger) renewDelegationAttempt(pod string, epoch int, now time.Time) (Delegation, error) {
	raw, exists, rows, err := l.delegationSnapshot()
	if err != nil {
		return Delegation{}, err
	}
	for i, d := range rows {
		if d.Pod != pod {
			continue
		}
		if d.Revoked {
			return Delegation{}, fmt.Errorf("maat: fenced: pod %q delegation revoked", pod)
		}
		if d.Epoch != epoch {
			return Delegation{}, fmt.Errorf("maat: fenced: epoch %d superseded (current %d)", epoch, d.Epoch)
		}
		rows[i].NotAfter = now.Add(maxDelegationTTL).Format(time.RFC3339)
		if err := l.delegationSave(raw, exists, rows); err != nil {
			return Delegation{}, err
		}
		return rows[i], nil
	}
	return Delegation{}, fmt.Errorf("maat: no delegation for pod %q", pod)
}

// AckDelegation records that the cloud has reconciled pod's journal through
// seq (ADR-120 §5 acked_seq) -- the write path for the field Delegation
// already carries but step 1 never wrote. Never regresses AckedSeq, so a
// catch-up replay's entries can be acked in any retry order. Does not touch
// Epoch or NotAfter.
func (l *Ledger) AckDelegation(pod string, seq int64) error {
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		err := l.ackDelegationAttempt(pod, seq)
		if !errors.Is(err, errLedgerConflict) {
			return err
		}
	}
	return fmt.Errorf("maat: ack delegation: contention limit: %w", errLedgerConflict)
}

func (l *Ledger) ackDelegationAttempt(pod string, seq int64) error {
	raw, exists, rows, err := l.delegationSnapshot()
	if err != nil {
		return err
	}
	for i, d := range rows {
		if d.Pod != pod {
			continue
		}
		if seq <= rows[i].AckedSeq {
			return nil
		}
		rows[i].AckedSeq = seq
		return l.delegationSave(raw, exists, rows)
	}
	return fmt.Errorf("maat: no delegation for pod %q", pod)
}

// GetDelegation returns pod's current delegation record (live or not), or
// nil if pod has never been delegated.
func (l *Ledger) GetDelegation(pod string) (*Delegation, error) {
	_, _, rows, err := l.delegationSnapshot()
	if err != nil {
		return nil, err
	}
	for _, d := range rows {
		if d.Pod == pod {
			dd := d
			return &dd, nil
		}
	}
	return nil, nil
}

// delegationFor returns the live delegation covering resource, or nil if
// resource is not currently delegated to anyone.
func (l *Ledger) delegationFor(resource string, now time.Time) (*Delegation, error) {
	_, _, rows, err := l.delegationSnapshot()
	if err != nil {
		return nil, err
	}
	for _, d := range rows {
		if d.expired(now) {
			continue
		}
		if d.coversResource(resource) {
			dd := d
			return &dd, nil
		}
	}
	return nil, nil
}

// UpsertReservation inserts req, or replaces the existing reservation with
// the same ID in place. The primitive step 2's catch-up (replaying a pod
// authority's offline grants) calls repeatedly: a retried replay after a
// crash rewrites the same row instead of adding a second (ADR-120 §5).
func (l *Ledger) UpsertReservation(req Reservation) (*Reservation, error) {
	if req.ID == "" {
		return nil, fmt.Errorf("maat: upsert requires a reservation id")
	}
	for attempt := 0; attempt < maxLedgerAttempts; attempt++ {
		out, err := l.upsertAttempt(req)
		if !errors.Is(err, errLedgerConflict) {
			return out, err
		}
	}
	return nil, fmt.Errorf("maat: upsert: contention limit: %w", errLedgerConflict)
}

func (l *Ledger) upsertAttempt(req Reservation) (*Reservation, error) {
	raw, exists, reservations, _, err := l.snapshot()
	if err != nil {
		return nil, err
	}
	replaced := false
	for i, r := range reservations {
		if r.ID == req.ID {
			reservations[i] = req
			replaced = true
			break
		}
	}
	if !replaced {
		reservations = append(reservations, req)
	}
	if err := l.save(raw, exists, reservations); err != nil {
		return nil, err
	}
	out := req
	return &out, nil
}

// ReserveAsAuthority is the pod authority's own write-through grant path
// (ADR-120 §2-3). It skips the normal admission/floor-share logic (the
// authority already enforced that locally) and checks only the fence: a
// live delegation naming req.Resource to req.Authority's pod, at exactly
// the epoch req was stamped with. Idempotent by req.ID via UpsertReservation,
// so a retried catch-up replay never creates a duplicate.
func (l *Ledger) ReserveAsAuthority(req Reservation) (ReserveResult, error) {
	if req.ID == "" {
		return ReserveResult{}, fmt.Errorf("maat: authority reservation requires an id")
	}
	if req.Authority == "" {
		return ReserveResult{}, fmt.Errorf("maat: authority reservation requires authority")
	}
	d, err := l.delegationFor(req.Resource, l.now())
	if err != nil {
		return ReserveResult{}, err
	}
	if d == nil {
		return ReserveResult{}, fmt.Errorf("maat: fenced: no live delegation for %s", req.Resource)
	}
	if "pod:"+d.Pod != req.Authority {
		return ReserveResult{}, fmt.Errorf("maat: fenced: %s does not hold %s (held by pod:%s)", req.Authority, req.Resource, d.Pod)
	}
	if req.Epoch != d.Epoch {
		return ReserveResult{}, fmt.Errorf("maat: fenced: epoch %d superseded (current %d)", req.Epoch, d.Epoch)
	}
	req.Status = StatusActive
	req.Share = ShareFull
	out, err := l.UpsertReservation(req)
	if err != nil {
		return ReserveResult{}, err
	}
	return ReserveResult{Granted: true, Reservation: out}, nil
}
