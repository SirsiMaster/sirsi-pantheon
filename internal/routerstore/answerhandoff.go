package routerstore

// Typed single-answerer state machine. Initial binding is native management
// only; a caller cannot bootstrap authority by naming a lane or a surface.
import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrAnswerHandoff = errors.New("routerstore: answering handoff refused")

// AnswerOwner is an exact native execution generation, not a heartbeat claim.
type AnswerOwner struct {
	ThreadID   string `json:"thread_id"`
	Host       string `json:"host"`
	MachineID  string `json:"machine_id"`
	Generation string `json:"generation"`
}

type AnswerHandoffCall struct {
	Op               string      `json:"op"`
	RequestID        string      `json:"request_id"`
	ExpectedEpoch    uint64      `json:"expected_epoch"`
	ExpectedRevision uint64      `json:"expected_revision"`
	ExpectedOwner    AnswerOwner `json:"expected_owner"`
	Target           AnswerOwner `json:"target"`
	SourceRef        string      `json:"source_ref"`
	DrainRef         string      `json:"drain_ref"`
	TTLSeconds       int         `json:"ttl_seconds"`
}

type AnswerHandoffEvent struct {
	Op        string      `json:"op"`
	RequestID string      `json:"request_id"`
	Epoch     uint64      `json:"epoch"`
	Actor     AnswerOwner `json:"actor"`
	At        string      `json:"at"`
	Evidence  string      `json:"evidence"`
}

type AnswerAssignment struct {
	Lane     string      `json:"lane"`
	Epoch    uint64      `json:"epoch"`
	Revision uint64      `json:"revision"`
	Owner    AnswerOwner `json:"owner"`
	Expires  string      `json:"expires"`
	// Enabled is false until the first cooperative activation. Legacy traffic is
	// unchanged while a seed/request is pending. Once enabled, expiry is a hold,
	// never an automatic fall back to legacy grants.
	Enabled bool                 `json:"enabled"`
	Revoked bool                 `json:"revoked"`
	Pending *AnswerHandoffCall   `json:"pending,omitempty"`
	Read    bool                 `json:"read"`
	Drained bool                 `json:"drained"`
	History []AnswerHandoffEvent `json:"history"`
}

func answerOwnerValid(a AnswerOwner) bool {
	return a.ThreadID != "" && a.Host != "" && a.MachineID != "" && a.Generation != ""
}
func answerLive(a AnswerAssignment, now time.Time) bool {
	t, e := time.Parse(time.RFC3339Nano, a.Expires)
	return e == nil && now.Before(t) && !a.Revoked
}
func answerTTL(n int) (time.Duration, error) {
	if n < 1 || n > 300 {
		return 0, fmt.Errorf("%w: lease must be 1..300 seconds", ErrAnswerHandoff)
	}
	return time.Duration(n) * time.Second, nil
}

// transitionAnswer is pure and never mutates the input on failure. Actor is
// supplied by authenticated backend context, NEVER deserialized from the call.
func transitionAnswer(in AnswerAssignment, actor AnswerOwner, c AnswerHandoffCall, now time.Time, drained bool) (AnswerAssignment, error) {
	fail := func(s string) (AnswerAssignment, error) { return in, fmt.Errorf("%w: %s", ErrAnswerHandoff, s) }
	if !answerOwnerValid(actor) {
		return fail("incomplete actual actor generation")
	}
	if c.Op == "read" {
		return in, nil
	}
	if len(in.History) >= 4096 {
		return fail("history capacity; retain records and reconcile")
	}
	if c.ExpectedRevision != in.Revision || c.ExpectedEpoch != in.Epoch || c.ExpectedOwner != in.Owner {
		return fail("expected owner/epoch changed")
	}
	// Lost-response mutations are deliberately fail closed by epoch/request CAS;
	// callers read exact retained state instead of replaying an uncertain write.
	if !answerLive(in, now) {
		return fail("owner expired or revoked; no automatic takeover")
	}
	out := in
	out.History = append([]AnswerHandoffEvent(nil), in.History...)
	evidence := c.SourceRef
	switch c.Op {
	case "request":
		if actor != in.Owner || in.Pending != nil || c.RequestID == "" || c.SourceRef == "" || !answerOwnerValid(c.Target) || c.Target == in.Owner {
			return fail("request requires current owner and distinct exact target")
		}
		if _, e := answerTTL(c.TTLSeconds); e != nil {
			return in, e
		}
		for _, e := range in.History {
			if e.RequestID == c.RequestID {
				return fail("request id already retained")
			}
		}
		pending := c
		out.Pending = &pending
		out.Read = false
		out.Drained = false
	case "accept", "ack-drain", "activate":
		if in.Pending == nil || c.RequestID != in.Pending.RequestID {
			return fail("exact pending request required")
		}
		switch c.Op {
		case "accept":
			if actor != in.Pending.Target || in.Read {
				return fail("target read acknowledgment required exactly once")
			}
			out.Read = true
		case "ack-drain":
			if actor != in.Owner || !in.Read || in.Drained || !drained || c.DrainRef == "" {
				return fail("current owner cooperative drain and no live work required")
			}
			out.Drained = true
			evidence = c.DrainRef
		case "activate":
			if actor != in.Pending.Target || !in.Read || !in.Drained || !drained {
				return fail("exact target and acknowledged drained owner required")
			}
			ttl, e := answerTTL(in.Pending.TTLSeconds)
			if e != nil {
				return in, e
			}
			out.Owner = actor
			out.Epoch++
			out.Enabled = true
			out.Expires = now.Add(ttl).Format(time.RFC3339Nano)
			out.Pending = nil
			out.Read = false
			out.Drained = false
		}
	case "renew":
		if actor != in.Owner {
			return fail("only current owner may renew")
		}
		ttl, e := answerTTL(c.TTLSeconds)
		if e != nil {
			return in, e
		}
		out.Expires = now.Add(ttl).Format(time.RFC3339Nano)
	case "revoke":
		if actor != in.Owner || c.SourceRef == "" {
			return fail("current owner evidence required")
		}
		out.Revoked = true
		out.Enabled = true
		out.Pending = nil
		out.Epoch++
	default:
		return fail("unknown operation")
	}
	out.Revision++
	out.History = append(out.History, AnswerHandoffEvent{Op: c.Op, RequestID: c.RequestID, Epoch: out.Epoch, Actor: actor, At: now.Format(time.RFC3339Nano), Evidence: evidence})
	return out, nil
}

const answerStatePrefix = "answer-handoff:v1:"

// withAnswerState serializes this lane across service replicas. Existing state
// table only; SQLite BEGIN IMMEDIATE and PostgreSQL row lock provide the same
// transaction boundary. No role is seeded implicitly and no DDL is needed.
func (s *SQLiteStore) withAnswerState(lane string, fn func(*txHandle, *AnswerAssignment) error) error {
	if lane == "" {
		return fmt.Errorf("%w: empty lane", ErrAnswerHandoff)
	}
	return s.withTx(func(tx *txHandle) error {
		key := answerStatePrefix + lane
		if s.dialect().name == "postgres" {
			var k string
			if err := tx.QueryRow(`SELECT key FROM state WHERE key=? FOR UPDATE`, key).Scan(&k); err != nil {
				return err
			}
		}
		raw, exists, err := (maatTransactionState{tx}).GetState(key)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: no management-bound owner", ErrAnswerHandoff)
		}
		var a AnswerAssignment
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return err
		}
		if a.Lane != lane || !answerOwnerValid(a.Owner) {
			return fmt.Errorf("%w: corrupt assignment", ErrAnswerHandoff)
		}
		if err = fn(tx, &a); err != nil {
			return err
		}
		b, err := json.Marshal(a)
		if err != nil {
			return err
		}
		return (maatTransactionState{tx}).SetState(key, string(b))
	})
}

// CreateAnswerAssignment is concrete backend management only (NOT Store/RPC).
// Deployment must bind a genuinely consenting current lead; no agent/session
// string, stale heartbeat, host token or cloud caller grants this privilege.
func (s *SQLiteStore) CreateAnswerAssignment(lane string, owner AnswerOwner, sourceRef string, ttl int) error {
	d, e := answerTTL(ttl)
	if e != nil {
		return e
	}
	if lane == "" || sourceRef == "" || !answerOwnerValid(owner) {
		return ErrAnswerHandoff
	}
	now := s.clock()
	a := AnswerAssignment{Lane: lane, Owner: owner, Expires: now.Add(d).Format(time.RFC3339Nano), History: []AnswerHandoffEvent{{Op: "seed", Actor: owner, At: now.Format(time.RFC3339Nano), Evidence: sourceRef}}}
	b, e := json.Marshal(a)
	if e != nil {
		return e
	}
	return s.withTx(func(tx *txHandle) error {
		r, e := tx.Exec(`INSERT INTO state(key,value) VALUES(?,?) ON CONFLICT(key) DO NOTHING`, answerStatePrefix+lane, string(b))
		if e != nil {
			return e
		}
		n, e := r.RowsAffected()
		if e != nil {
			return e
		}
		if n != 1 {
			return fmt.Errorf("%w: assignment already exists", ErrAnswerHandoff)
		}
		return nil
	})
}

// applyAnswerHandoff is not exported as a forgeable actor RPC. The transport
// adapter must derive actor from the authenticated session and enrolled host.
func (s *SQLiteStore) applyAnswerHandoff(lane string, actor AnswerOwner, c AnswerHandoffCall) (AnswerAssignment, error) {
	var out AnswerAssignment
	err := s.withAnswerState(lane, func(tx *txHandle, a *AnswerAssignment) error {
		// Both drain ACK and activation recheck durable executable work in the SAME
		// transaction. A later integrated claim gate must share this state row lock.
		var tasks, items int
		now := s.clock()
		if c.Op == "ack-drain" || c.Op == "activate" {
			if err := tx.QueryRow(`SELECT COUNT(*) FROM tasks WHERE agent=? AND thread_id=? AND status='in-progress' AND lease_token<>''`, lane, a.Owner.ThreadID).Scan(&tasks); err != nil {
				return err
			}
			// Item lease session binding is the authoritative owning session. Do not
			// reconstruct or export old lease secrets to identify the owner.
			if err := tx.QueryRow(`SELECT COUNT(*) FROM items i JOIN lease_sessions ls ON ls.kind='item' AND ls.key=i.id JOIN sessions s ON ls.session=s.session_id WHERE i.to_agent=? AND s.thread_id=? AND i.lease_token<>'' AND i.status<>'closed'`, lane, a.Owner.ThreadID).Scan(&items); err != nil {
				return err
			}
		}
		next, err := transitionAnswer(*a, actor, c, now, tasks == 0 && items == 0)
		if err != nil {
			return err
		}
		*a = next
		out = next
		return nil
	})
	return out, err
}

type answerActorKey struct{}
type answerActor struct {
	Session   Session
	MachineID string
}

// answerActorOwner resolves the actual stored execution generation under the
// transaction. Caller-provided host/thread/generation is never trusted.
func answerActorOwner(tx *txHandle, actor answerActor, now time.Time) (AnswerOwner, error) {
	var status, agent, host, last string
	var payload []byte
	err := tx.QueryRow(`SELECT status,agent,host,last_seen_at,payload FROM threads WHERE thread_id=?`, actor.Session.ThreadID).Scan(&status, &agent, &host, &last, &payload)
	if err != nil {
		return AnswerOwner{}, err
	}
	at, e := time.Parse(time.RFC3339Nano, last)
	if e != nil || status != "active" || agent != actor.Session.Agent || host != actor.Session.Host || now.Sub(at) > 90*time.Second || at.After(now.Add(5*time.Second)) || actor.MachineID == "" {
		return AnswerOwner{}, ErrAnswerHandoff
	}
	var p struct {
		PID       int    `json:"pid"`
		Start     string `json:"start_time"`
		MachineID string `json:"machine_id"`
	}
	if json.Unmarshal(payload, &p) != nil || p.PID <= 0 || p.Start == "" || p.MachineID != actor.MachineID {
		return AnswerOwner{}, ErrAnswerHandoff
	}
	var revoked string
	if err = tx.QueryRow(`SELECT revoked FROM sessions WHERE session_id=? AND agent=? AND host=? AND thread_id=?`, actor.Session.ID, agent, host, actor.Session.ThreadID).Scan(&revoked); err != nil || revoked != "" {
		return AnswerOwner{}, ErrAnswerHandoff
	}
	return AnswerOwner{ThreadID: actor.Session.ThreadID, Host: host, MachineID: actor.MachineID, Generation: answerGeneration(actor.MachineID, actor.Session.ID, p.PID, p.Start)}, nil
}

// answerGeneration binds ownership to the server-minted session as well as the
// self-reported process fields, so a client that copies a thread's PID and start
// time still cannot produce the owner's generation. A new session is a new
// generation: the old owner's authority does not follow it.
func answerGeneration(machineID, sessionID string, pid int, start string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s/%s/%d/%s", machineID, sessionID, pid, start))))
}

// AnswerHandoff is a self-lane control operation. A read reveals pending work,
// never acts as consent. The transport alone supplies authenticated actor.
func (s *SQLiteStore) AnswerHandoff(ctx context.Context, lane string, c AnswerHandoffCall) (AnswerAssignment, error) {
	if c.Op == "activate" {
		return AnswerAssignment{}, fmt.Errorf("%w: draft activation disabled until atomic execution gates qualify", ErrAnswerHandoff)
	}
	actor, ok := ctx.Value(answerActorKey{}).(answerActor)
	if !ok || actor.Session.Agent != lane || actor.Session.ThreadID == "" {
		return AnswerAssignment{}, ErrAnswerHandoff
	}
	var out AnswerAssignment
	err := s.withAnswerState(lane, func(tx *txHandle, a *AnswerAssignment) error {
		who, err := answerActorOwner(tx, actor, s.clock())
		if err != nil {
			return err
		}
		// An unrelated same-lane session cannot inspect candidate identities.
		if who != a.Owner && (a.Pending == nil || who != a.Pending.Target) {
			return ErrAnswerHandoff
		}
		var tasks, items int
		if c.Op == "ack-drain" || c.Op == "activate" {
			if err = tx.QueryRow(`SELECT COUNT(*) FROM tasks WHERE agent=? AND thread_id=? AND status='in-progress' AND lease_token<>''`, lane, a.Owner.ThreadID).Scan(&tasks); err != nil {
				return err
			}
			if err = tx.QueryRow(`SELECT COUNT(*) FROM items i JOIN lease_sessions ls ON ls.kind='item' AND ls.key=i.id JOIN sessions s ON ls.session=s.session_id WHERE i.to_agent=? AND s.thread_id=? AND i.lease_token<>'' AND i.status<>'closed'`, lane, a.Owner.ThreadID).Scan(&items); err != nil {
				return err
			}
		}
		next, err := transitionAnswer(*a, who, c, s.clock(), tasks == 0 && items == 0)
		if err != nil {
			return err
		}
		*a = next
		out = next
		return nil
	})
	return out, err
}
