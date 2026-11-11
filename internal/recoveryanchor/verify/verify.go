package verify

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"
)

// VerbIdentity is a verb's build-time-approved identity (ADR-078 §3): the
// fixed launchd label or volume UUID, and the pinned config digest. The
// caller supplies it; this package never learns it from a request.
type VerbIdentity struct {
	Target       string
	ConfigDigest string
}

// Config configures a Verifier. Every field is required.
type Config struct {
	NodeID  string                // this node's locally pinned identity
	Verbs   map[Verb]VerbIdentity // approved identity per fixed verb
	MaxTTL  time.Duration         // server-bounded maximum, not caller-supplied
	MaxSkew time.Duration         // bounded clock-skew allowance
	Clock   Clock
	Store   Store
}

// Verifier decides admission. It holds no state of its own.
type Verifier struct{ cfg Config }

// NewVerifier validates cfg. A bad config is an error, never a permissive
// default.
func NewVerifier(cfg Config) (*Verifier, error) {
	switch {
	case !reNodeID.MatchString(cfg.NodeID):
		return nil, errors.New("verify: NodeID missing or malformed")
	case cfg.MaxTTL <= 0 || cfg.MaxSkew < 0:
		return nil, errors.New("verify: MaxTTL must be positive and MaxSkew non-negative")
	case cfg.Clock == nil || cfg.Store == nil:
		return nil, errors.New("verify: Clock and Store are required")
	}
	for v, id := range cfg.Verbs {
		if !IsVerb(v) || !reTarget.MatchString(id.Target) || !reDigest.MatchString(id.ConfigDigest) {
			return nil, fmt.Errorf("verify: bad approved identity for verb %q", v)
		}
	}
	return &Verifier{cfg: cfg}, nil
}

// Request is one candidate request as received. PeerUID is the kernel
// peer-credential UID the caller read from the (future) socket; this package
// only compares it.
type Request struct {
	Envelope  []byte // exact canonical bytes that were signed
	Signature []byte // armored SSHSIG over Envelope
	PeerUID   uint32
}

// Admit decides whether req would be admitted under policy pol. It is the
// whole Verifier + atomic-admission + Authorizer path of ADR-078's data flow,
// minus the pieces that execute (custody fence, Executor, audit), which are
// not in this package. On allow it has atomically reserved the request in the
// Store; on any deny the Store is unchanged. Fails closed on every error,
// including a recovered panic.
func (v *Verifier) Admit(req Request, pol *Policy) (d Decision) {
	defer func() {
		if r := recover(); r != nil {
			d = denied(ReasonInternal, "recovered panic")
		}
	}()
	if pol == nil {
		return denied(ReasonPolicyInvalid, "no policy snapshot")
	}
	env, err := ParseEnvelope(req.Envelope)
	if err != nil {
		return deniedErr(err)
	}
	sig, err := ParseSignature(req.Signature)
	if err != nil {
		return deniedErr(err)
	}
	now := v.cfg.Clock.Now()
	var out Decision
	uerr := v.cfg.Store.Update(func(tx Tx) error {
		out = v.admitTx(tx, req, env, sig, pol, now)
		return nil
	})
	if uerr != nil {
		return denied(ReasonStoreError, "store update failed; admission not durable")
	}
	return out
}

func (v *Verifier) admitTx(tx Tx, req Request, env Envelope, sig *Signature, pol *Policy, now time.Time) Decision {
	switch cur := tx.Epoch(); {
	case env.Epoch < cur:
		return denied(ReasonEpochRetired, "envelope epoch is retired")
	case env.Epoch > cur:
		return denied(ReasonEpochFuture, "envelope epoch is not current")
	}
	grant, ok := pol.Lookup(sig.Fingerprint)
	if !ok {
		return denied(ReasonUnknownSigner, "signing key has no active grant")
	}
	if err := sig.VerifyMessage(req.Envelope); err != nil {
		return deniedErr(err)
	}
	if req.PeerUID != grant.UID {
		return denied(ReasonUIDMismatch, "presenting UID is not the signing key's mapped UID")
	}
	if env.NodeID != v.cfg.NodeID {
		return denied(ReasonWrongNode, "request is for a different node")
	}
	if !IsVerb(env.Verb) {
		return denied(ReasonVerbUnknown, "verb is outside the fixed set")
	}
	id, ok := v.cfg.Verbs[env.Verb]
	switch {
	case !ok:
		return denied(ReasonVerbNotConfigured, "verb has no approved identity on this node")
	case env.Target != id.Target:
		return denied(ReasonTargetMismatch, "target is not the verb's approved identity")
	case env.ConfigDigest != id.ConfigDigest:
		return denied(ReasonDigestMismatch, "config_digest is not the verb's approved digest")
	}
	nowU := now.Unix()
	switch {
	case env.ExpiresAt <= env.IssuedAt:
		return denied(ReasonTTLInvalid, "expires_at is not after issued_at")
	case env.ExpiresAt-env.IssuedAt > skewSeconds(v.cfg.MaxTTL):
		return denied(ReasonTTLTooLong, "TTL exceeds the server-bounded maximum")
	case env.IssuedAt > nowU+skewSeconds(v.cfg.MaxSkew):
		return denied(ReasonNotYetValid, "issued_at is beyond the skew allowance")
	case nowU >= env.ExpiresAt:
		return denied(ReasonExpired, "expires_at has passed")
	}
	// ponytail: ADR §1a orders the Authorizer after reservation. Checking the
	// grant here means an unauthorized verb never burns a replay row; the
	// outcome (deny) is identical. Revisit only if audit needs the row.
	if !grant.Allows(env.Verb) {
		return denied(ReasonVerbNotGranted, "verb is not in the signing key's grant")
	}

	// Atomic admission: everything below reads, decides, then writes once.
	if rolledBack(nowU, tx.Ratchet(), v.cfg.MaxSkew) {
		return denied(ReasonClockRollback, "clock reads behind ratchet minus max skew")
	}
	hash := sha256.Sum256(req.Envelope)
	if prev, found := tx.Reservation(sig.Fingerprint, env.RequestID); found {
		if prev == hash {
			return denied(ReasonReplaySame, "request_id already reserved with identical content")
		}
		return denied(ReasonReplayConflict, "request_id already reserved with different content")
	}
	if env.IssuedAt < tx.Mark() {
		return denied(ReasonBehindMark, "issued_at is behind this epoch's high-water mark")
	}
	tx.Reserve(sig.Fingerprint, env.RequestID, hash, env.ExpiresAt)
	if env.IssuedAt > tx.Mark() {
		tx.SetMark(env.IssuedAt)
	}
	advanceRatchet(tx, nowU)
	return Decision{
		Allowed: true,
		Reason:  ReasonAdmitted,
		Admitted: &Admitted{
			Envelope:    env,
			Fingerprint: sig.Fingerprint,
			UID:         req.PeerUID,
			ContentHash: hash,
			Generation:  pol.Generation,
			AdmittedAt:  now,
		},
	}
}

// Current is everything Revalidate needs, gathered by the caller UNDER ITS
// SHARED GUARD immediately before dispatch (ADR-078 §1a). This package has no
// guard and does not read any file; assembling Current correctly under the
// guard is the future daemon's job.
type Current struct {
	Epoch               uint64   // daemon's current protected epoch
	RatchetFloor        int64    // current epoch's last_observed_wallclock
	Dirty               bool     // sync step's dirty flag
	PublishedGeneration uint64   // current published generation
	PublishedHashes     HashPair // hashes the published generation derived from
	// Live is the result of the synchronous in-guard BuildPolicy run NOW on
	// the current source bytes; LiveErr is its error. If LiveErr != nil or
	// Live == nil, revalidation denies.
	Live    *Policy
	LiveErr error
}

// Revalidate is the pure pre-dispatch check. It re-runs epoch, dirty,
// content-hash, ratchet, expiry, generation-fence and grant checks against
// the CURRENT state, never a snapshot from admission. Allowed means only "no
// reason to deny at this instant"; the linearization point is the caller's
// executor invocation under its guard, which this package never performs.
func Revalidate(a *Admitted, cur Current, now time.Time, maxSkew time.Duration) Decision {
	if a == nil {
		return denied(ReasonInternal, "no admitted request")
	}
	switch {
	case a.Envelope.Epoch < cur.Epoch:
		return denied(ReasonEpochRetired, "epoch retired since admission")
	case a.Envelope.Epoch > cur.Epoch:
		return denied(ReasonEpochFuture, "epoch is not current")
	case cur.Dirty:
		return denied(ReasonPolicyDirty, "policy is dirty; last-known-good does not apply")
	case cur.LiveErr != nil || cur.Live == nil:
		return denied(ReasonPolicyInvalid, "synchronous validate failed")
	case cur.Live.Hashes != cur.PublishedHashes:
		return denied(ReasonPolicyDirty, "source content differs from the published generation")
	}
	nowU := now.Unix()
	switch {
	case rolledBack(nowU, cur.RatchetFloor, maxSkew):
		return denied(ReasonClockRollback, "clock reads behind ratchet minus max skew")
	case nowU >= a.Envelope.ExpiresAt:
		return denied(ReasonExpired, "expires_at has passed")
	case cur.PublishedGeneration != a.Generation:
		return denied(ReasonGenerationMoved, "policy generation advanced since admission")
	}
	g, ok := cur.Live.Lookup(a.Fingerprint)
	if !ok || g.UID != a.UID || !g.Allows(a.Envelope.Verb) {
		return denied(ReasonGrantRevoked, "grant removed or narrowed since admission")
	}
	return Decision{Allowed: true, Reason: ReasonRevalidated}
}
