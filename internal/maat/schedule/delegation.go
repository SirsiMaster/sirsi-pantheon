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

// AtomicDelegationScheduler keeps delegation and reservation writes under
// the canonical reservation transaction. Older clients without this optional
// feature keep their existing undelegated scheduler path.
type AtomicDelegationScheduler interface {
	MaatDelegation(action string, req Delegation) (Delegation, error)
	MaatGetDelegation(pod string) (*Delegation, error)
	MaatAuthorityReserve(machine string, req Reservation) (ReserveResult, error)
	MaatReconcile(machine, pod string, epoch int, rows []Reservation) (int64, error)
}

const delegationStateKey = "maat:delegations"

// maxDelegationTTL is the finite window cap (ADR-120 §4): a delegation never
// renews itself past this without the canonical ledger being reachable.
const maxDelegationTTL = 12 * time.Hour

// A partitioned authority may retain its original lease after cloud revocation.
const delegationHandbackMargin = 5 * time.Minute

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

// blocksCanonical is intentionally independent of Revoked: a cloud-side bit
// cannot revoke an offline cached lease. Invalid retained expiry fails closed.
// Earlier handback requires a real native fence acknowledgement (not yet wired).
func (d Delegation) blocksCanonical(now time.Time) bool {
	end, err := time.Parse(time.RFC3339, d.NotAfter)
	return err != nil || now.Before(end.Add(delegationHandbackMargin))
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
	if a, ok := l.store.(AtomicDelegationScheduler); ok {
		return a.MaatDelegation("delegate", req)
	}
	if _, canonical := l.store.(AtomicScheduler); canonical {
		return Delegation{}, fmt.Errorf("maat: canonical store lacks atomic delegation RPC; upgrade compatible server first")
	}
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
	// Delegating a resource with outstanding canonical work would create two
	// authorities even if all delegation-key writes were otherwise serialized.
	_, _, active, _, err := l.snapshot()
	if err != nil {
		return Delegation{}, err
	}
	for _, row := range active {
		if row.Status == StatusActive {
			for _, resource := range req.Resources {
				if sameResource(row.Resource, resource) {
					return Delegation{}, fmt.Errorf("maat: resource %q has active canonical reservation %q", resource, row.ID)
				}
			}
		}
	}
	nextEpoch := 1
	replaceAt := -1
	for i, d := range rows {
		if d.Pod == req.Pod {
			if d.blocksCanonical(now) {
				return Delegation{}, fmt.Errorf("maat: pod %q handback pending through old expiry plus margin", d.Pod)
			}
			replaceAt = i
			nextEpoch = d.Epoch + 1
			continue
		}
		if !d.blocksCanonical(now) {
			continue
		}
		for _, r := range req.Resources {
			if d.coversResource(r) {
				return Delegation{}, fmt.Errorf("maat: resource %q already delegated to pod %q (epoch %d)", r, d.Pod, d.Epoch)
			}
		}
	}
	req.Epoch = nextEpoch
	req.AckedSeq = 0
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

// Revoke stops canonical acceptance of pod grants immediately, but cannot
// revoke an offline cache. Canonical/new-authority admission remains fenced
// through the original expiry plus handback margin.
func (l *Ledger) Revoke(pod string) (Delegation, error) {
	if a, ok := l.store.(AtomicDelegationScheduler); ok {
		return a.MaatDelegation("revoke", Delegation{Pod: pod})
	}
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

// GetDelegation returns pod's current delegation record (live or not), or
// nil if pod has never been delegated.
func (l *Ledger) GetDelegation(pod string) (*Delegation, error) {
	if a, ok := l.store.(AtomicDelegationScheduler); ok {
		return a.MaatGetDelegation(pod)
	}
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
		if !d.blocksCanonical(now) {
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
			// A replay is not a new owner and cannot resurrect terminal work.
			if r.Resource != req.Resource || r.Holder != req.Holder || r.Authority != req.Authority || r.Epoch != req.Epoch {
				return nil, fmt.Errorf("maat: reconcile reservation %q ownership/epoch mismatch", req.ID)
			}
			if req.Seq < r.Seq {
				return nil, fmt.Errorf("maat: stale reservation replay sequence")
			}
			switch r.Status {
			case StatusReleased, StatusExpired, StatusInvalidated, StatusDone:
				if req.Status != r.Status {
					return nil, fmt.Errorf("maat: terminal reservation cannot be resurrected by replay")
				}
			}
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
	if a, ok := l.store.(AtomicDelegationScheduler); ok {
		return a.MaatAuthorityReserve("", req)
	}
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
	if d == nil || d.expired(l.now()) {
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

// ReconcileAuthority runs only inside the canonical transaction adapter. The
// machine is derived by the authenticated service handler, never trusted from
// wire input. Each batch and its acknowledgement commit or roll back together.
func (l *Ledger) ReconcileAuthority(machine, pod string, epoch int, batch []Reservation) (int64, error) {
	if len(batch) > 256 {
		return 0, fmt.Errorf("maat: replay batch exceeds 256")
	}
	raw, exists, ds, err := l.delegationSnapshot()
	if err != nil {
		return 0, err
	}
	at := -1
	for i, d := range ds {
		if d.Pod == pod {
			at = i
			break
		}
	}
	if at < 0 {
		return 0, fmt.Errorf("maat: unknown delegation")
	}
	d := ds[at]
	if machine == "" || machine != d.AuthorityMachine || epoch != d.Epoch {
		return 0, fmt.Errorf("maat: replay authority/epoch mismatch")
	}
	for _, req := range batch {
		if req.Authority != "pod:"+pod || req.Epoch != epoch || req.Seq < 1 || !d.coversResource(req.Resource) {
			return 0, fmt.Errorf("maat: invalid replay ownership/sequence")
		}
		if req.Seq <= d.AckedSeq {
			_, _, known, _, e := l.snapshot()
			if e != nil {
				return 0, e
			}
			found := false
			for _, old := range known {
				if old.ID == req.ID && old.Authority == req.Authority && old.Epoch == req.Epoch && old.Seq >= req.Seq && old.Holder == req.Holder && old.Resource == req.Resource {
					found = true
					break
				}
			}
			if !found {
				return 0, fmt.Errorf("maat: replay sequence aliases unknown/different reservation")
			}
			continue
		}
		if req.Seq != d.AckedSeq+1 {
			return 0, fmt.Errorf("maat: replay sequence gap")
		}
		if d.expired(l.now()) && req.Status == StatusActive {
			return 0, fmt.Errorf("maat: expired/revoked authority cannot replay active grants")
		}
		if _, e := l.UpsertReservation(req); e != nil {
			return 0, e
		}
		d.AckedSeq = req.Seq
	}
	ds[at] = d
	if err := l.delegationSave(raw, exists, ds); err != nil {
		return 0, err
	}
	return d.AckedSeq, nil
}

// CheckLegacyReservationWrite preserves the compatible CAS path for resources
// not delegated, while checking changed rows under the same canonical lock.
func (l *Ledger) CheckLegacyReservationWrite(oldRaw, newRaw string) error {
	var oldDoc, newDoc struct {
		Reservations []Reservation `json:"reservations"`
	}
	if oldRaw != "" {
		if err := json.Unmarshal([]byte(oldRaw), &oldDoc); err != nil {
			return err
		}
	}
	if err := json.Unmarshal([]byte(newRaw), &newDoc); err != nil {
		return err
	}
	previous := map[string]Reservation{}
	for _, r := range oldDoc.Reservations {
		previous[r.ID] = r
	}
	for _, r := range newDoc.Reservations {
		before, exists := previous[r.ID]
		delete(previous, r.ID)
		a, _ := json.Marshal(before)
		b, _ := json.Marshal(r)
		if exists && string(a) == string(b) {
			continue
		}
		if r.Authority != "" || before.Authority != "" {
			return fmt.Errorf("maat: legacy writes cannot create or alter pod authority grants")
		}
		if d, err := l.delegationFor(r.Resource, l.now()); err != nil {
			return err
		} else if d != nil {
			return fmt.Errorf("maat: legacy write changes delegated resource %q", r.Resource)
		}
	}
	for _, r := range previous {
		if d, err := l.delegationFor(r.Resource, l.now()); err != nil {
			return err
		} else if d != nil {
			return fmt.Errorf("maat: legacy write removes delegated resource %q", r.Resource)
		}
	}
	return nil
}
