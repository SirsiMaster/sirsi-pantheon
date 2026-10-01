package routerstore

// ADR-062 §3 (rs-10): identity is a registered agent SESSION, never a cwd.
//
// A session is minted by the service (MintSession) for a (host, agent,
// runtime) triple and carries a secret the node uses to sign every request.
// Ownership: at claim time the service binds the session to the item/task
// (BindItemSession/BindTaskSession); every lease-bearing mutation must come
// from that same session (ItemSession/TaskSession) — a lease token alone is
// not enough, because a token can be copied and a session key cannot be
// presented without its signer.

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
)

// Session is what the service knows about one registered agent runtime.
type Session struct {
	ID          string `json:"id"`
	Secret      string `json:"secret,omitempty"` // only returned by MintSession
	Host        string `json:"host"`
	Agent       string `json:"agent"`
	RuntimeHash string `json:"runtime_hash"`
	ThreadID    string `json:"thread_id,omitempty"` // the registered thread this session is bound to (Rule of Ra)
	Created     string `json:"created"`
	LastSeen    string `json:"last_seen"`
	Revoked     string `json:"revoked,omitempty"`
}

var (
	ErrSessionUnknown = errors.New("routerstore: session unknown")
	ErrSessionRevoked = errors.New("routerstore: session revoked")
	ErrNotOwner       = errors.New("routerstore: caller's session does not own this lease")
)

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// MintSession creates a session bound to (host, agent, runtimeHash). The
// secret is returned exactly once.
func (s *SQLiteStore) MintSession(host, agent, runtimeHash string) (Session, error) {
	return s.MintSessionForThread(host, agent, runtimeHash, "")
}

// MintSessionForThread mints a session bound to a registered thread (the Rule
// of Ra, ADR-062 20b.1). threadID may be empty for legacy callers; the service
// then treats every mutation from that session as unregistered.
//
// When threadID is non-empty, an existing live session already bound to the
// exact same (host, agent, runtimeHash, threadID) is reused instead of
// minting a fresh one. A sandboxed consumer (fork/exec denied, so it cannot
// run `ps`) re-invokes the CLI as a brand new OS process for every verb —
// claim, then complete, then release — and each process's client-side
// session cache can go missing independently (sandboxed-consumer-session-
// binding, ra ledger 2026-10-01). Without reuse, every such process mints a
// distinct session id/secret, so the session a task was claimed under can
// never match the session attempting to complete or release it, and every
// fenced mutation fails with ErrNotOwner even though it is the same
// registered thread throughout. Reuse makes the registered thread — not the
// client's local cache — the source of session identity.
func (s *SQLiteStore) MintSessionForThread(host, agent, runtimeHash, threadID string) (Session, error) {
	if host == "" || agent == "" || runtimeHash == "" {
		return Session{}, errors.New("routerstore: MintSession: host, agent and runtime hash are all required")
	}
	if threadID != "" {
		if existing, ok, err := s.liveSessionForThread(host, agent, runtimeHash, threadID); err != nil {
			return Session{}, fmt.Errorf("routerstore: MintSession: %w", err)
		} else if ok {
			_ = s.TouchSession(existing.ID)
			return existing, nil
		}
	}
	id, err := randomHex(16)
	if err != nil {
		return Session{}, err
	}
	secret, err := randomHex(32)
	if err != nil {
		return Session{}, err
	}
	now := s.clock().Format(time.RFC3339)
	if _, err := s.exec(`INSERT INTO sessions(session_id,secret,host,agent,runtime_hash,thread_id,created,last_seen,revoked)
		VALUES(?,?,?,?,?,?,?,?,'')`, id, secret, host, agent, runtimeHash, threadID, now, now); err != nil {
		return Session{}, fmt.Errorf("routerstore: MintSession: %w", err)
	}
	return Session{ID: id, Secret: secret, Host: host, Agent: agent, RuntimeHash: runtimeHash, ThreadID: threadID, Created: now, LastSeen: now}, nil
}

// liveSessionForThread returns the most recent non-revoked session already
// minted for the exact (host, agent, runtimeHash, threadID) tuple, including
// its secret, so MintSessionForThread can hand the caller back the same
// session it (or an earlier process acting as the same registered thread)
// already holds. ok is false when none exists.
func (s *SQLiteStore) liveSessionForThread(host, agent, runtimeHash, threadID string) (Session, bool, error) {
	var sess Session
	err := s.db.QueryRow(`SELECT session_id,secret,host,agent,runtime_hash,thread_id,created,last_seen
		FROM sessions
		WHERE host=? AND agent=? AND runtime_hash=? AND thread_id=? AND revoked=''
		ORDER BY created DESC LIMIT 1`, host, agent, runtimeHash, threadID).
		Scan(&sess.ID, &sess.Secret, &sess.Host, &sess.Agent, &sess.RuntimeHash, &sess.ThreadID, &sess.Created, &sess.LastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	return sess, true, nil
}

// GetSession returns the session including its secret (server-side use only:
// the handler needs the secret to verify signatures). ErrSessionUnknown or
// ErrSessionRevoked otherwise.
func (s *SQLiteStore) GetSession(id string) (Session, error) {
	var sess Session
	err := s.db.QueryRow(`SELECT session_id,secret,host,agent,runtime_hash,thread_id,created,last_seen,revoked FROM sessions WHERE session_id=?`, id).
		Scan(&sess.ID, &sess.Secret, &sess.Host, &sess.Agent, &sess.RuntimeHash, &sess.ThreadID, &sess.Created, &sess.LastSeen, &sess.Revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrSessionUnknown
	}
	if err != nil {
		return Session{}, fmt.Errorf("routerstore: GetSession: %w", err)
	}
	if sess.Revoked != "" {
		return Session{}, ErrSessionRevoked
	}
	return sess, nil
}

// RevokeSession makes a session unusable on its next request.
func (s *SQLiteStore) RevokeSession(id string) error {
	res, err := s.exec(`UPDATE sessions SET revoked=? WHERE session_id=? AND revoked=''`, s.clock().Format(time.RFC3339), id)
	if err != nil {
		return fmt.Errorf("routerstore: RevokeSession: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrSessionUnknown
	}
	return nil
}

// TouchSession records liveness.
func (s *SQLiteStore) TouchSession(id string) error {
	_, err := s.exec(`UPDATE sessions SET last_seen=? WHERE session_id=?`, s.clock().Format(time.RFC3339), id)
	return err
}

// BindItemSession records which session holds the item's current lease.
func (s *SQLiteStore) BindItemSession(id, session string) error {
	_, err := s.exec(`INSERT INTO lease_sessions(kind,key,session) VALUES('item',?,?)
		ON CONFLICT(kind,key) DO UPDATE SET session=excluded.session`, id, session)
	return err
}

// ItemSession returns the session bound to the item; empty when unbound (a
// local claim, or an item that predates sessions — ownership is then open).
func (s *SQLiteStore) ItemSession(id string) (string, error) {
	var sess string
	err := s.db.QueryRow(`SELECT session FROM lease_sessions WHERE kind='item' AND key=?`, id).Scan(&sess)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil // unbound: a local claim, or an item that predates sessions
	}
	return sess, err
}

// BindTaskSession / TaskSession are the ledger-task twins.
func (s *SQLiteStore) BindTaskSession(agent, taskID, session string) error {
	_, err := s.exec(`INSERT INTO lease_sessions(kind,key,session) VALUES('task',?,?)
		ON CONFLICT(kind,key) DO UPDATE SET session=excluded.session`, agent+"/"+taskID, session)
	return err
}

func (s *SQLiteStore) TaskSession(agent, taskID string) (string, error) {
	var sess string
	err := s.db.QueryRow(`SELECT session FROM lease_sessions WHERE kind='task' AND key=?`, agent+"/"+taskID).Scan(&sess)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return sess, err
}
