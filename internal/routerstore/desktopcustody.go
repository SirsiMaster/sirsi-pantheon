package routerstore

// Sleep policy is application data under the existing service-management
// boundary. No bearer, lane name, RPC, or local grant file creates authority.
import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	dc "github.com/SirsiMaster/sirsi-pantheon/internal/desktopcustody"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/schedule"
)

const desktopSleepKey = "desktop-custody:sleep:v1"

type desktopSleepActorKey struct{}
type desktopSleepActor struct{ Participant dc.Participant }
type desktopSleepRegistry struct{ Records map[string]dc.CustodyState }

func sleepDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func sleepSHA(v string) bool {
	b, e := hex.DecodeString(v)
	return e == nil && len(b) == 32 && v == strings.ToLower(v)
}

func sleepRole(d dc.Delegation, role string) dc.Participant {
	switch role {
	case "coordinator":
		return d.Coordinator
	case "custodian":
		return d.Custodian
	case "observer":
		return d.Observer
	}
	return dc.Participant{}
}

func validateSleepDelegation(d dc.Delegation) error {
	a, r := d.Admission, d.Request
	if (d.CustodianReservationID == "") != (d.CustodianWork == "") {
		return errors.New("sleep custody: controller reservation and work must be bound together")
	}
	if a.Operation != dc.SleepOperation || a.ID == "" || a.ID != r.ID || a.RequestID != r.ID || a.Host != r.Host || a.Issuer != "sirsi-router" || a.Owner == "" || a.PlatformUUID == "" || a.BootID == "" || a.Nonce == "" || a.CustodyGeneration == "" || !sleepSHA(a.ReviewDigest) || d.OwnerRequestRef == "" || !sleepSHA(d.OwnerRequestSHA256) || !sleepSHA(d.ExecutableSHA256) || !sleepSHA(d.CandidateSHA256) || !sleepSHA(d.InventorySHA256) {
		return errors.New("sleep custody: incomplete finite management binding")
	}
	if a.NotBefore.IsZero() || !a.NotAfter.After(a.NotBefore) || a.NotAfter.Sub(a.NotBefore) > 30*time.Minute || !r.WakeAt.After(a.NotBefore) || !r.WakeAt.Before(a.NotAfter) || r.WakeAt.Nanosecond() != 0 {
		return errors.New("sleep custody: finite window/wake invalid")
	}
	seen := map[string]bool{}
	for role, p := range []dc.Participant{d.Coordinator, d.Custodian, d.Observer} {
		// An existing cloud coordinator is bound to its exact logical host
		// token/session; it need not acquire a fabricated hardware UUID. Both
		// native participants must have their actual adopted machine identity.
		if p.TokenID == "" || p.SessionID == "" || (role != 0 && p.MachineID == "") || p.Host == "" || p.Agent == "" || p.ThreadID == "" || !sleepSHA(p.RuntimeSHA256) || seen[p.SessionID] {
			return errors.New("sleep custody: exact distinct participants required")
		}
		seen[p.SessionID] = true
	}
	if d.Custodian.MachineID != a.PlatformUUID || d.Custodian.RuntimeSHA256 != d.ExecutableSHA256 || d.Observer.MachineID == d.Custodian.MachineID || d.Observer.Host == d.Custodian.Host || d.ObserverMachineLabel != "m1" || d.Coordinator.MachineID == d.Custodian.MachineID || d.Coordinator.Host == d.Custodian.Host {
		return errors.New("sleep custody: native custodian or independent M1 observer mismatch")
	}
	return nil
}

// participantLive runs inside the same transaction as policy changes. The
// stored session secret never leaves the normal authentication implementation.
func participantLive(tx *txHandle, p dc.Participant, now time.Time) error {
	var host, mid, revoked string
	if err := tx.QueryRow(`SELECT host,machine_id,revoked FROM host_tokens WHERE token_id=?`, p.TokenID).Scan(&host, &mid, &revoked); err != nil {
		return err
	}
	if host != p.Host || mid != p.MachineID || revoked != "" {
		return errors.New("sleep custody: enrolled participant token changed or revoked")
	}
	var s Session
	if err := tx.QueryRow(`SELECT session_id,host,agent,runtime_hash,thread_id,revoked FROM sessions WHERE session_id=?`, p.SessionID).Scan(&s.ID, &s.Host, &s.Agent, &s.RuntimeHash, &s.ThreadID, &s.Revoked); err != nil {
		return err
	}
	if s.Host != p.Host || s.Agent != p.Agent || s.RuntimeHash != p.RuntimeSHA256 || s.ThreadID != p.ThreadID || s.Revoked != "" {
		return errors.New("sleep custody: participant session changed or revoked")
	}
	var agent, status, last, threadHost string
	if err := tx.QueryRow(`SELECT agent,status,last_seen_at,host FROM threads WHERE thread_id=?`, p.ThreadID).Scan(&agent, &status, &last, &threadHost); err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, last)
	if err != nil || agent != p.Agent || threadHost != p.Host || status == "closed" || status == "suspended" || now.Sub(at) > 90*time.Second || at.After(now.Add(5*time.Second)) {
		return errors.New("sleep custody: participant thread not fresh and live")
	}
	return nil
}

func readSleepRegistry(tx *txHandle) (desktopSleepRegistry, error) {
	v, exists, err := (maatTransactionState{tx}).GetState(desktopSleepKey)
	r := desktopSleepRegistry{Records: map[string]dc.CustodyState{}}
	if err != nil || !exists {
		return r, err
	}
	if err = json.Unmarshal([]byte(v), &r); err != nil {
		return r, err
	}
	if r.Records == nil {
		return r, errors.New("sleep custody: corrupt canonical registry")
	}
	for id, v := range r.Records {
		if id != v.Delegation.Request.ID || v.Ready == nil {
			return r, errors.New("sleep custody: corrupt retained record")
		}
		if err := validateSleepDelegation(v.Delegation); err != nil {
			return r, err
		}
	}
	return r, nil
}
func writeSleepRegistry(tx *txHandle, r desktopSleepRegistry) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return (maatTransactionState{tx}).SetState(desktopSleepKey, string(b))
}

// CreateDesktopSleep is concrete-backend management ONLY: deliberately absent
// from Store and therefore absent from reflection RPC and RemoteStore. Exact
// replay at revision startup is idempotent; no terminal record is reset.
func (s *SQLiteStore) CreateDesktopSleep(d dc.Delegation) error {
	if err := validateSleepDelegation(d); err != nil {
		return err
	}
	return s.withMaatTransaction(func(tx *txHandle, _ *schedule.Ledger) error {
		r, err := readSleepRegistry(tx)
		if err != nil {
			return err
		}
		if old, ok := r.Records[d.Request.ID]; ok {
			if sleepDigest(old.Delegation) == sleepDigest(d) {
				return nil
			}
			return errors.New("sleep custody: management cannot replace an existing request")
		}
		if len(r.Records) >= 1024 {
			return errors.New("sleep custody: retained policy capacity reached; audited maintenance required")
		}
		for _, old := range r.Records {
			if old.Delegation.Admission.Nonce == d.Admission.Nonce {
				return errors.New("sleep custody: nonce already retained")
			}
		}
		now := s.clock()
		if !d.Admission.NotAfter.After(now) {
			return errors.New("sleep custody: cannot create expired delegation")
		}
		for _, p := range []dc.Participant{d.Coordinator, d.Custodian, d.Observer} {
			if err := participantLive(tx, p, now); err != nil {
				return err
			}
		}
		r.Records[d.Request.ID] = dc.CustodyState{Delegation: d, Ready: map[string]dc.CustodyReady{}}
		return writeSleepRegistry(tx, r)
	})
}

// RevokeDesktopSleep cannot remove a consumed host fence; only independently
// verified return can clear it. Revocation never manufactures an idle machine.
func (s *SQLiteStore) RevokeDesktopSleep(id, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return errors.New("sleep custody: revocation reason required")
	}
	return s.withMaatTransaction(func(tx *txHandle, _ *schedule.Ledger) error {
		r, err := readSleepRegistry(tx)
		if err != nil {
			return err
		}
		v, ok := r.Records[id]
		if !ok {
			return ErrNotFound
		}
		if v.RevokedAt.IsZero() {
			v.RevokedAt = s.clock()
			v.RevocationReason = reason
			r.Records[id] = v
		}
		return writeSleepRegistry(tx, r)
	})
}

func (s *SQLiteStore) DesktopSleep(ctx context.Context, c dc.CustodyCall) (dc.CustodyState, error) {
	actor, ok := ctx.Value(desktopSleepActorKey{}).(desktopSleepActor)
	if !ok {
		return dc.CustodyState{}, errors.New("sleep custody: verified service actor required")
	}
	var out dc.CustodyState
	err := s.withMaatTransaction(func(tx *txHandle, l *schedule.Ledger) error {
		r, err := readSleepRegistry(tx)
		if err != nil {
			return err
		}
		v, ok := r.Records[c.ID]
		if !ok {
			return ErrNotFound
		}
		d, a := v.Delegation, v.Delegation.Admission
		now := s.clock()
		if actor.Participant != sleepRole(d, c.Role) {
			return errors.New("sleep custody: actor is not the exact delegated participant")
		}
		if err := participantLive(tx, actor.Participant, now); err != nil {
			return err
		}
		if c.Operation == "status" {
			out = v
			return nil
		}
		if !v.RevokedAt.IsZero() && c.Operation != "returned" && c.Operation != "observed-return" {
			return errors.New("sleep custody: delegation revoked")
		}
		if c.Operation != "returned" && c.Operation != "observed-return" && (now.Before(a.NotBefore) || !now.Before(a.NotAfter) || !v.ConsumedAt.IsZero()) {
			return errors.New("sleep custody: delegation expired, not active, or already consumed")
		}
		if c.Role == "custodian" && c.Operation != "returned" {
			want := dc.HostIdentity{Host: a.Host, PlatformUUID: a.PlatformUUID, BootID: a.BootID, ExecutableSHA256: d.ExecutableSHA256}
			if c.Identity != want || c.InventorySHA256 != d.InventorySHA256 {
				return errors.New("sleep custody: actual boot, executable or workload inventory changed")
			}
		}
		switch c.Operation {
		case "inspect":
		case "ready":
			digest := c.QuietDigest
			if c.Role == "custodian" {
				digest = sleepDigest(struct{ Inventory, Generation string }{c.InventorySHA256, a.CustodyGeneration})
			}
			if !sleepSHA(digest) {
				return errors.New("sleep custody: actual custody digest required")
			}
			v.Ready[c.Role] = dc.CustodyReady{At: now, Digest: digest}
		case "consume":
			if c.Role != "custodian" {
				return errors.New("sleep custody: only native custodian consumes")
			}
			for _, role := range []string{"coordinator", "custodian", "observer"} {
				p := sleepRole(d, role)
				if err := participantLive(tx, p, now); err != nil {
					return err
				}
				ready, ok := v.Ready[role]
				if !ok || now.Sub(ready.At) > 15*time.Second || ready.At.After(now) {
					return errors.New("sleep custody: fresh three-party custody required")
				}
			}
			if err := sleepReserveFence(tx, l, a.Host); err != nil {
				return err
			}
			rows, err := l.Status("")
			if err != nil {
				return err
			}
			for _, row := range rows {
				if d.CustodianReservationID != "" && row.ID == d.CustodianReservationID && row.Holder == d.Custodian.Agent && row.Work == d.CustodianWork && row.Status == schedule.StatusActive {
					continue
				}
				end, e1 := time.Parse(time.RFC3339, row.EstEnd)
				start, e2 := time.Parse(time.RFC3339, row.Start)
				if sleepHostResource(d, row.Resource) && (row.Status == schedule.StatusActive || row.Status == schedule.StatusQueued) && (e1 != nil || e2 != nil || (end.After(now) && start.Before(a.NotAfter))) {
					return errors.New("sleep custody: current or scheduled workload reservation overlaps window")
				}
			}
			v.ConsumedAt = now
		case "observed-return":
			if c.Role != "observer" || v.ConsumedAt.IsZero() || c.Observation == nil {
				return errors.New("sleep custody: consumed request and independent observer required")
			}
			e := *c.Observation
			if e.RequestID != c.ID || e.Host != a.Host || e.PlatformUUID != a.PlatformUUID || e.BootID != a.BootID || e.LostAt.Before(v.ConsumedAt) || !e.ReturnedAt.After(e.LostAt) || e.ReturnedAt.Before(d.Request.WakeAt) || e.ReturnedAt.After(now.Add(5*time.Second)) || now.Sub(e.ReturnedAt) > 90*time.Second || !sleepSHA(e.ContinuitySHA256) {
				return errors.New("sleep custody: invalid independently observed continuity interval")
			}
			if v.Observation != nil && *v.Observation != e {
				return errors.New("sleep custody: observer evidence cannot be replaced")
			}
			v.Observation = &e
		case "returned":
			// A return report cannot be accepted on the strength of a caller's
			// JSON alone. The independent observer must first attest the SAME
			// evidence through its separately authenticated session.
			if c.Role != "custodian" || v.ConsumedAt.IsZero() || c.Return == nil || v.Observation == nil || now.Before(d.Request.WakeAt) {
				return errors.New("sleep custody: return before actual consumed window")
			}
			e := *c.Return
			if e.RequestID != c.ID || e.Host != a.Host || e.BootID != a.BootID || e.PlatformUUID != a.PlatformUUID || !e.FullWake || e.SleepRecordID == "" || e.WakeRecordID != "pantheon-sleep-"+c.ID || e.ObservedAt.Before(d.Request.WakeAt) || e.ObservedAt.After(now.Add(5*time.Second)) || !sleepSHA(e.ObserverReceiptDigest) || !sleepSHA(e.Digest) {
				return errors.New("sleep custody: invalid full-wake/continuity evidence")
			}
			if e.ObserverReceiptDigest != sleepDigest(*v.Observation) || e.ObservedAt.After(v.Observation.ReturnedAt.Add(5*time.Second)) {
				return errors.New("sleep custody: independent observer does not bind full wake")
			}
			if v.Return != nil && *v.Return != e {
				return errors.New("sleep custody: verified return cannot be replaced")
			}
			v.Return = &e
		default:
			return errors.New("sleep custody: unsupported typed operation")
		}
		r.Records[c.ID] = v
		out = v
		return writeSleepRegistry(tx, r)
	})
	return out, err
}

func sleepReserveFence(tx *txHandle, _ *schedule.Ledger, resource string) error {
	r, err := readSleepRegistry(tx)
	if err != nil {
		return err
	}
	for _, v := range r.Records {
		if !v.ConsumedAt.IsZero() && v.Return == nil && sleepHostResource(v.Delegation, resource) {
			return fmt.Errorf("sleep custody: unresolved consumed host fence %s", v.Delegation.Request.ID)
		}
	}
	return nil
}

func sleepHostResource(d dc.Delegation, resource string) bool {
	for _, host := range []string{d.Admission.Host, d.Custodian.Host, d.Custodian.MachineID} {
		if resource == host || strings.HasSuffix(resource, "@"+host) {
			return true
		}
	}
	return false
}
