package authority

import (
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

// Authority is the pod-local booking authority a live delegation names
// (ADR-120 §2-5). It runs ordinary schedule.Ledger admission (floor share,
// cedes, conflicts unchanged) against its own LOCAL ledger, journals every
// grant, and write-throughs to the canonical (cloud) ledger when reachable
// via Ledger.ReserveAsAuthority.
//
// One instance is good for exactly one (pod, epoch) pair: a superseded or
// revoked epoch fences it permanently (ADR-120 §3) -- a new epoch needs a
// fresh Authority built from the new delegation, not a reset of this one.
type Authority struct {
	Pod   string
	id    string // "pod:<pod>", the Reservation.Authority stamp
	Local *schedule.Ledger
	Cloud *schedule.Ledger // nil: cloud unreachable; Reserve still admits+journals, no write-through

	journal Journal
	now     func() time.Time

	epoch    int
	notAfter time.Time
	ackedSeq int64
	fenced   bool
}

// New returns an Authority for delegation d. cloud may be nil (constructed
// while offline); SetCloud attaches it once reachable.
func New(d schedule.Delegation, local, cloud *schedule.Ledger, journal Journal) (*Authority, error) {
	notAfter, err := time.Parse(time.RFC3339, d.NotAfter)
	if err != nil {
		return nil, fmt.Errorf("maat: authority: delegation not_after %q: %w", d.NotAfter, err)
	}
	return &Authority{
		Pod:      d.Pod,
		id:       "pod:" + d.Pod,
		Local:    local,
		Cloud:    cloud,
		journal:  journal,
		now:      time.Now,
		epoch:    d.Epoch,
		notAfter: notAfter,
		ackedSeq: d.AckedSeq,
	}, nil
}

// WithClock overrides the clock (tests).
func (a *Authority) WithClock(now func() time.Time) *Authority { a.now = now; return a }

// SetCloud attaches (or replaces) the cloud ledger once reachable.
func (a *Authority) SetCloud(cloud *schedule.Ledger) { a.Cloud = cloud }

// Fenced reports whether this instance has stopped granting: expired, or a
// Renew saw itself superseded or revoked (ADR-120 §3-4). Permanent in the
// superseded/revoked case -- a fenced Authority never un-fences itself; a new
// epoch needs a new instance.
func (a *Authority) Fenced() bool { return a.fenced || !a.now().Before(a.notAfter) }

// Reserve admits req through the ordinary Local ledger (floor share, cedes,
// conflicts unchanged), then journals and write-throughs the grant stamped
// with this Authority's id/epoch (ADR-120 §2-3). Refuses outright once
// Fenced(): an expired or superseded authority grants nothing, so the caller
// fails closed exactly as ADR-120 requires.
func (a *Authority) Reserve(req schedule.Reservation, queue bool) (schedule.ReserveResult, error) {
	if a.Fenced() {
		return schedule.ReserveResult{}, fmt.Errorf("maat: fenced: pod authority %s (epoch %d) not granting", a.id, a.epoch)
	}
	result, err := a.Local.Reserve(req, queue)
	if err != nil || !result.Granted || result.Reservation == nil {
		return result, err
	}
	stamped := *result.Reservation
	stamped.Authority = a.id
	stamped.Epoch = a.epoch
	entry, err := a.journal.Append(stamped)
	if err != nil {
		return schedule.ReserveResult{}, fmt.Errorf("maat: journal grant: %w", err)
	}
	stamped.Seq = entry.Seq
	result.Reservation = &stamped
	if a.Cloud != nil {
		// A write-through failure here (the cloud dropped between the call
		// starting and now) never undoes a grant already handed to the
		// caller: the entry is already durable in the journal, so CatchUp
		// reconciles it once the cloud is reachable again (ADR-120 §5).
		if _, werr := a.Cloud.ReserveAsAuthority(stamped); werr == nil {
			a.ackedSeq = entry.Seq
			_ = a.Cloud.AckDelegation(a.Pod, entry.Seq)
		}
	}
	return result, nil
}

// Renew is the periodic (every 15 min, ADR-120 §4) check-in: it asks the
// cloud to extend this delegation's window to now+12h, which doubles as the
// self-fence check (ADR-120 §3) -- RenewDelegation itself refuses a revoked
// or epoch-superseded delegation, and that refusal permanently fences this
// instance. A transient cloud-unreachable error does NOT fence: the
// delegation's existing (not yet expired) not_after still holds until Fenced
// observes real expiry.
func (a *Authority) Renew() error {
	if a.Cloud == nil {
		return fmt.Errorf("maat: authority %s: no cloud ledger to renew against", a.id)
	}
	d, err := a.Cloud.RenewDelegation(a.Pod, a.epoch, a.now())
	if err != nil {
		if strings.Contains(err.Error(), "fenced:") {
			a.fenced = true
		}
		return err
	}
	notAfter, err := time.Parse(time.RFC3339, d.NotAfter)
	if err != nil {
		return fmt.Errorf("maat: authority: renewed not_after %q: %w", d.NotAfter, err)
	}
	a.notAfter = notAfter
	return nil
}

// CatchUp replays every journaled grant the cloud has not yet acknowledged
// (Seq > acked_seq) as an idempotent upsert (ADR-120 §5). Safe to call after
// a crash between a previous upsert and its ack: ReserveAsAuthority's
// UpsertReservation is keyed by reservation ID, so replaying an
// already-applied entry rewrites the same row, never a second one.
func (a *Authority) CatchUp() error {
	if a.Cloud == nil {
		return fmt.Errorf("maat: authority %s: no cloud ledger to catch up to", a.id)
	}
	entries, err := a.journal.Since(a.ackedSeq)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if _, err := a.Cloud.ReserveAsAuthority(e.Reservation); err != nil {
			return fmt.Errorf("maat: catch-up seq %d: %w", e.Seq, err)
		}
		if err := a.Cloud.AckDelegation(a.Pod, e.Seq); err != nil {
			return fmt.Errorf("maat: catch-up ack seq %d: %w", e.Seq, err)
		}
		a.ackedSeq = e.Seq
	}
	return nil
}
