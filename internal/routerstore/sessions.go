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

// Session GC (task ra-session-table-gc). A session is one row per mint and nothing ever removed them: ~86k rows in 48 h from
// one Mac. Dead sessions are deleted opportunistically on mint (Cloud Run has no scheduler), at most once per interval and in
// bounded batches, and never a session that still owns a lease on an open item or an unfinished task.
const (
	sessionGCIdle     = 24 * time.Hour
	sessionGCInterval = 5 * time.Minute
	sessionGCLimit    = 5000
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
func (s *SQLiteStore) MintSessionForThread(host, agent, runtimeHash, threadID string) (Session, error) {
	if host == "" || agent == "" || runtimeHash == "" {
		return Session{}, errors.New("routerstore: MintSession: host, agent and runtime hash are all required")
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
	s.maybeGCSessions()
	return Session{ID: id, Secret: secret, Host: host, Agent: agent, RuntimeHash: runtimeHash, ThreadID: threadID, Created: now, LastSeen: now}, nil
}

// maybeGCSessions runs GCSessions when one is due. Best effort: a failed GC must never fail a mint.
func (s *SQLiteStore) maybeGCSessions() {
	now := s.clock()
	s.sessionGCMu.Lock()
	due := !now.Before(s.sessionGCNext)
	if due {
		s.sessionGCNext = now.Add(sessionGCInterval)
	}
	s.sessionGCMu.Unlock()
	if due {
		_, _ = s.GCSessions(sessionGCIdle, sessionGCLimit)
	}
}

// GCSessions deletes up to limit dead sessions and returns how many it removed. Dead means: not revoked and last seen more
// than idle ago, or revoked more than idle ago. A session is KEPT while it owns a lease (lease_sessions) on an item that is
// not closed/completed/dead_letter or a task that is not done: ownership checks compare the caller's session to that binding.
// lease_sessions rows are never deleted, so a binding to a finished item protects nothing. The client re-mints once on
// "session unknown", so deleting an idle session is safe for a caller that comes back.
func (s *SQLiteStore) GCSessions(idle time.Duration, limit int) (int64, error) {
	if limit < 1 {
		limit = sessionGCLimit
	}
	cut := s.clock().Add(-idle).Format(time.RFC3339)
	res, err := s.exec(`DELETE FROM sessions WHERE session_id IN (
		SELECT s2.session_id FROM sessions s2 WHERE `+sessionDeadPredicate+` ORDER BY s2.last_seen LIMIT ?)`, cut, cut, limit)
	if err != nil {
		return 0, fmt.Errorf("routerstore: GCSessions: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// sessionDeadPredicate selects dead sessions (see GCSessions). Args: idle cutoff twice.
const sessionDeadPredicate = `((s2.revoked = '' AND s2.last_seen < ?) OR (s2.revoked <> '' AND s2.revoked < ?))
	  AND NOT EXISTS (SELECT 1 FROM lease_sessions ls JOIN items i ON ls.kind = 'item' AND ls.key = i.id
	                  WHERE ls.session = s2.session_id AND i.status NOT IN ('closed','completed','dead_letter'))
	  AND NOT EXISTS (SELECT 1 FROM lease_sessions ls JOIN tasks t ON ls.kind = 'task' AND ls.key = t.agent || '/' || t.task_id
	                  WHERE ls.session = s2.session_id AND t.status <> 'done')`

// SessionStats is the operator's view of the sessions table.
type SessionStats struct {
	Total, Live, Revoked, Idle, Collectable int64
	ByHost                                  []HostSessions
}

// HostSessions is one host's row count.
type HostSessions struct {
	Host  string
	Count int64
}

// SessionStats counts the table: Live = unrevoked and seen within the hour, Idle = unrevoked and unseen for idle, Collectable =
// what GCSessions(idle) would delete now (no limit). Read-only.
func (s *SQLiteStore) SessionStats(idle time.Duration) (SessionStats, error) {
	var st SessionStats
	now := s.clock()
	cut := now.Add(-idle).Format(time.RFC3339)
	live := now.Add(-time.Hour).Format(time.RFC3339)
	one := func(dst *int64, q string, args ...any) error {
		if err := s.db.QueryRow(q, args...).Scan(dst); err != nil {
			return fmt.Errorf("routerstore: SessionStats: %w", err)
		}
		return nil
	}
	for _, c := range []struct {
		dst  *int64
		q    string
		args []any
	}{
		{&st.Total, `SELECT count(*) FROM sessions`, nil},
		{&st.Revoked, `SELECT count(*) FROM sessions WHERE revoked <> ''`, nil},
		{&st.Live, `SELECT count(*) FROM sessions WHERE revoked = '' AND last_seen >= ?`, []any{live}},
		{&st.Idle, `SELECT count(*) FROM sessions WHERE revoked = '' AND last_seen < ?`, []any{cut}},
		{&st.Collectable, `SELECT count(*) FROM sessions s2 WHERE ` + sessionDeadPredicate, []any{cut, cut}},
	} {
		if err := one(c.dst, c.q, c.args...); err != nil {
			return st, err
		}
	}
	rows, err := s.db.Query(`SELECT host, count(*) FROM sessions GROUP BY host ORDER BY 2 DESC LIMIT 8`)
	if err != nil {
		return st, fmt.Errorf("routerstore: SessionStats: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var h HostSessions
		if err := rows.Scan(&h.Host, &h.Count); err != nil {
			return st, err
		}
		st.ByHost = append(st.ByHost, h)
	}
	return st, rows.Err()
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
