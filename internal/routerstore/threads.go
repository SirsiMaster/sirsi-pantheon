package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ThreadRecord is the store-owned envelope for a CTR thread payload.
type ThreadRecord struct {
	ThreadID   string
	Agent      string
	Status     string
	LastSeenAt string
	Payload    []byte
	Host       string // the machine the thread runs on (Rule of Ra binding); "" for legacy rows
}

// ThreadBinding is what the Rule of Ra gate needs about a thread.
type ThreadBinding struct {
	ThreadID   string `json:"thread_id"`
	Agent      string `json:"agent"`
	Host       string `json:"host"`
	Status     string `json:"status"`
	LastSeenAt string `json:"last_seen_at"`
}

// ErrThreadUnknown: no registered thread with that id.
var ErrThreadUnknown = errors.New("routerstore: thread not registered")

// ThreadBinding returns the gate view of one thread; ErrThreadUnknown when
// there is no such row.
func (s *SQLiteStore) ThreadBinding(threadID string) (ThreadBinding, error) {
	var b ThreadBinding
	err := s.db.QueryRow(`SELECT thread_id,agent,host,status,last_seen_at FROM threads WHERE thread_id=?`, threadID).
		Scan(&b.ThreadID, &b.Agent, &b.Host, &b.Status, &b.LastSeenAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ThreadBinding{}, ErrThreadUnknown
	}
	if err != nil {
		return ThreadBinding{}, fmt.Errorf("routerstore: ThreadBinding: %w", err)
	}
	return b, nil
}

// ImportThreadsIfEmpty atomically seeds the store at cutover. A non-empty
// table is already authoritative and is never blended with a stale JSON file.
func (s *SQLiteStore) ImportThreadsIfEmpty(records []ThreadRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("routerstore: begin thread import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM threads`).Scan(&count); err != nil {
		return fmt.Errorf("routerstore: count threads: %w", err)
	}
	if count > 0 {
		return tx.Commit()
	}
	for _, r := range records {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO threads(thread_id,agent,status,last_seen_at,payload,host) VALUES(?,?,?,?,?,?)`, r.ThreadID, r.Agent, r.Status, r.LastSeenAt, r.Payload, r.Host); err != nil {
			return fmt.Errorf("routerstore: import thread %q: %w", r.ThreadID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("routerstore: commit thread import: %w", err)
	}
	return nil
}

// UpsertThreads applies row-scoped lifecycle mutations. It never deletes rows
// absent from the caller's snapshot, so two processes registering distinct
// threads cannot erase one another. Terminal and suspended states win over a
// stale active/idle/blocked heartbeat, preventing late liveness from reviving
// an explicitly closed, reaped, or parked session.
func (s *SQLiteStore) UpsertThreads(records []ThreadRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("routerstore: begin thread upsert: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, r := range records {
		if _, err := tx.Exec(`
INSERT INTO threads(thread_id,agent,status,last_seen_at,payload,host) VALUES(?,?,?,?,?,?)
ON CONFLICT(thread_id) DO UPDATE SET
 agent=excluded.agent,status=excluded.status,last_seen_at=excluded.last_seen_at,payload=excluded.payload,
 host=CASE WHEN excluded.host='' THEN threads.host ELSE excluded.host END
WHERE threads.status NOT IN ('closed','reaped','suspended')
  AND (threads.host='' OR excluded.host='' OR threads.host=excluded.host)
  AND (excluded.last_seen_at > threads.last_seen_at
       OR excluded.last_seen_at = threads.last_seen_at
          AND (excluded.status IN ('closed','reaped','suspended')
               OR excluded.status = threads.status AND excluded.payload > threads.payload))`, r.ThreadID, r.Agent, r.Status, r.LastSeenAt, r.Payload, r.Host); err != nil {
			return fmt.Errorf("routerstore: upsert thread %q: %w", r.ThreadID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("routerstore: commit thread upsert: %w", err)
	}
	return nil
}

// UpsertThreadCAS applies one explicitly mutated lifecycle row and reports
// whether its fence accepted the write. Snapshot reconciliation uses
// UpsertThreads, where an unrelated stale row is an expected no-op; lifecycle
// verbs use this method so losing their own target fence fails loudly.
func (s *SQLiteStore) UpsertThreadCAS(r ThreadRecord) (bool, error) {
	result, err := s.db.Exec(`
INSERT INTO threads(thread_id,agent,status,last_seen_at,payload,host) VALUES(?,?,?,?,?,?)
ON CONFLICT(thread_id) DO UPDATE SET
 agent=excluded.agent,status=excluded.status,last_seen_at=excluded.last_seen_at,payload=excluded.payload,
 host=CASE WHEN excluded.host='' THEN threads.host ELSE excluded.host END
WHERE threads.status NOT IN ('closed','reaped','suspended')
  AND (threads.host='' OR excluded.host='' OR threads.host=excluded.host)
  AND (excluded.last_seen_at > threads.last_seen_at
       OR excluded.last_seen_at = threads.last_seen_at
          AND (excluded.status IN ('closed','reaped','suspended')
               OR excluded.status = threads.status AND excluded.payload > threads.payload))`,
		r.ThreadID, r.Agent, r.Status, r.LastSeenAt, r.Payload, r.Host)
	if err != nil {
		return false, fmt.Errorf("routerstore: upsert thread %q: %w", r.ThreadID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("routerstore: count upsert thread %q: %w", r.ThreadID, err)
	}
	return affected == 1, nil
}

// ResumeThreadCAS is the sole suspended-to-active transition. Requiring the
// exact suspended row observed by the caller prevents a heartbeat that loaded
// before a concurrent suspend from implicitly resuming the thread.
func (s *SQLiteStore) ResumeThreadCAS(record ThreadRecord, suspendedAt string) error {
	result, err := s.db.Exec(`UPDATE threads
SET agent=?,status=?,last_seen_at=?,payload=?,host=CASE WHEN ?='' THEN host ELSE ? END
WHERE thread_id=? AND status='suspended' AND last_seen_at=? AND (host='' OR ?='' OR host=?)`,
		record.Agent, record.Status, record.LastSeenAt, record.Payload, record.Host, record.Host, record.ThreadID, suspendedAt, record.Host, record.Host)
	if err != nil {
		return fmt.Errorf("routerstore: resume thread %q: %w", record.ThreadID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("routerstore: count resumed thread %q: %w", record.ThreadID, err)
	}
	if affected != 1 {
		return fmt.Errorf("routerstore: resume thread %q lost suspended-state fence", record.ThreadID)
	}
	return nil
}

// Host ownership (the Rule of Ra, SSA 2026-09-10 r2): every mutation above
// and below carries the caller's host INSIDE its predicate — a row on another
// host is never rewritten, resumed or deleted, and two hosts adopting one
// blank legacy row cannot both win: the first commit sets host, the second
// statement's predicate fails. A blank caller host ('' — a local file store
// with no host identity; the service always stamps the session host) passes
// through unchanged.

// DeleteThreadCAS removes only the exact row observed by the pruning read,
// and only when that row is on the caller's host.
// A concurrent heartbeat/status transition changes last_seen/status and makes
// this a safe no-op rather than deleting live truth.
func (s *SQLiteStore) DeleteThreadCAS(threadID, status, lastSeenAt, host string) (bool, error) {
	result, err := s.db.Exec(`DELETE FROM threads WHERE thread_id=? AND status=? AND last_seen_at=? AND (host='' OR ?='' OR host=?)`, threadID, status, lastSeenAt, host, host)
	if err != nil {
		return false, fmt.Errorf("routerstore: delete thread %q: %w", threadID, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("routerstore: count deleted thread %q: %w", threadID, err)
	}
	return affected == 1, nil
}

// ListThreads returns every durable thread payload.
func (s *SQLiteStore) ListThreads() ([]ThreadRecord, error) {
	rows, err := s.db.Query(`SELECT thread_id,agent,status,last_seen_at,payload,host FROM threads ORDER BY thread_id`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: list threads: %w", err)
	}
	defer rows.Close()
	var out []ThreadRecord
	for rows.Next() {
		var r ThreadRecord
		if err := rows.Scan(&r.ThreadID, &r.Agent, &r.Status, &r.LastSeenAt, &r.Payload, &r.Host); err != nil {
			return nil, fmt.Errorf("routerstore: scan thread: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routerstore: iterate threads: %w", err)
	}
	return out, nil
}

// AudienceEntry is one gated call as the Rule of Ra saw it.
type AudienceEntry struct {
	TS        string `json:"ts"`
	Method    string `json:"method"`
	SessionID string `json:"session_id"`
	Agent     string `json:"agent"`
	Host      string `json:"host"`
	ThreadID  string `json:"thread_id"`
	Verdict   string `json:"verdict"` // allowed | would_refuse | refused
	Reason    string `json:"reason,omitempty"`
}

// AudienceTS is the fixed-width UTC timestamp the audience log stores
// (RFC3339Nano is variable-width, so "…:00Z" would sort after "…:00.5Z").
const AudienceTS = "2006-01-02T15:04:05.000000000Z07:00"

// NormalizeAudienceTS renders any RFC3339 instant in the log's fixed width;
// a value that does not parse is returned unchanged.
func NormalizeAudienceTS(ts string) string {
	if t, err := time.Parse(time.RFC3339Nano, ts); err == nil {
		return t.UTC().Format(AudienceTS)
	}
	return ts
}

// RecordAudience appends one gate verdict (ADR-062 20b.3).
func (s *SQLiteStore) RecordAudience(e AudienceEntry) error {
	if e.TS == "" {
		e.TS = s.clock().UTC().Format(AudienceTS)
	} else {
		e.TS = NormalizeAudienceTS(e.TS)
	}
	_, err := s.exec(`INSERT INTO audience_log(ts,method,session_id,agent,host,thread_id,verdict,reason) VALUES(?,?,?,?,?,?,?,?)`,
		e.TS, e.Method, e.SessionID, e.Agent, e.Host, e.ThreadID, e.Verdict, e.Reason)
	if err != nil {
		return fmt.Errorf("routerstore: RecordAudience: %w", err)
	}
	return nil
}

// AudienceReport is the 20b.3 audit, two separate questions:
//
//	(a) history — every gated call since `since` as the gate saw it AT THAT
//	    MOMENT (Gated/Allowed/Failures/ByAgent); the log row is written before
//	    the mutation runs and a write failure refuses the call, so a
//	    mutation without a row does not exist;
//	(b) live coverage — sessions seen since `since`, not revoked, that carry no
//	    thread (Unbound), read from the sessions table, independent of (a).
//
// Recorded is false when the window holds no rows at all: "nothing recorded"
// is not "everything allowed", and the CLI says so. Mode is filled by the
// server (the store does not know the gate mode).
type AudienceReport struct {
	Since    string          `json:"since"`
	Mode     string          `json:"mode,omitempty"`
	Recorded bool            `json:"recorded"`
	Gated    int             `json:"gated"`
	Allowed  int             `json:"allowed"`
	Failures []AudienceEntry `json:"failures"` // would_refuse | refused
	ByAgent  map[string]int  `json:"failures_by_agent"`
	Unbound  []string        `json:"live_sessions_without_thread"` // "agent@host session" — live sessions with no thread
}

// AudienceSince builds the report over audience_log rows with ts >= since
// (fixed-width comparison) and the live session population.
func (s *SQLiteStore) AudienceSince(since string) (AudienceReport, error) {
	// `since` reaches here over the generic remote API, so validate before any
	// slice/query — a malformed value is a normal error, never a panic (SSA
	// 2026-09-10, PR #726 r2 P2).
	t, perr := time.Parse(time.RFC3339Nano, strings.TrimSpace(since))
	if perr != nil {
		return AudienceReport{}, fmt.Errorf("routerstore: AudienceSince: invalid since %q (want RFC3339): %w", since, perr)
	}
	t = t.UTC()
	sinceLog := t.Format(AudienceTS) // fixed-width, for the nanosecond audience_log
	// Sessions store last_seen at whole-second RFC3339. A fractional `since`
	// (e.g. 15:00:00.5Z) must EXCLUDE a session at the previous whole second
	// (15:00:00Z is the instant 15:00:00.0, before .5), so ceil the threshold
	// to the next whole second when `since` has a sub-second part.
	sinceSec := t.Truncate(time.Second)
	if t.After(sinceSec) {
		sinceSec = sinceSec.Add(time.Second)
	}
	sinceSess := sinceSec.Format(time.RFC3339)
	rep := AudienceReport{Since: sinceLog, ByAgent: map[string]int{}, Failures: []AudienceEntry{}, Unbound: []string{}}
	rows, err := s.db.Query(`SELECT ts,method,session_id,agent,host,thread_id,verdict,reason FROM audience_log WHERE ts >= ? ORDER BY ts`, sinceLog)
	if err != nil {
		return rep, fmt.Errorf("routerstore: AudienceSince: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var e AudienceEntry
		if scanErr := rows.Scan(&e.TS, &e.Method, &e.SessionID, &e.Agent, &e.Host, &e.ThreadID, &e.Verdict, &e.Reason); scanErr != nil {
			return rep, scanErr
		}
		rep.Recorded = true
		rep.Gated++
		if e.Verdict == "allowed" {
			rep.Allowed++
			continue
		}
		rep.Failures = append(rep.Failures, e)
		rep.ByAgent[e.Agent]++
	}
	if rerr := rows.Err(); rerr != nil {
		return rep, rerr
	}
	// (b) live coverage: the session table itself. Session timestamps are
	// RFC3339 (second precision); compare on the same width.
	live, err := s.db.Query(`SELECT session_id,agent,host FROM sessions WHERE revoked='' AND thread_id='' AND last_seen >= ? ORDER BY agent,host,session_id`, sinceSess)
	if err != nil {
		return rep, fmt.Errorf("routerstore: AudienceSince sessions: %w", err)
	}
	defer func() { _ = live.Close() }()
	for live.Next() {
		var id, agent, host string
		if scanErr := live.Scan(&id, &agent, &host); scanErr != nil {
			return rep, scanErr
		}
		rep.Unbound = append(rep.Unbound, agent+"@"+host+" "+id)
	}
	return rep, live.Err()
}
