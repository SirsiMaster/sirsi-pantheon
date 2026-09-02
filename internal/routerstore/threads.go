package routerstore

import "fmt"

// ThreadRecord is the store-owned envelope for a CTR thread payload.
type ThreadRecord struct {
	ThreadID   string
	Agent      string
	Status     string
	LastSeenAt string
	Payload    []byte
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
		if _, err := tx.Exec(`INSERT OR IGNORE INTO threads(thread_id,agent,status,last_seen_at,payload) VALUES(?,?,?,?,?)`, r.ThreadID, r.Agent, r.Status, r.LastSeenAt, r.Payload); err != nil {
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
INSERT INTO threads(thread_id,agent,status,last_seen_at,payload) VALUES(?,?,?,?,?)
ON CONFLICT(thread_id) DO UPDATE SET
 agent=excluded.agent,status=excluded.status,last_seen_at=excluded.last_seen_at,payload=excluded.payload
WHERE threads.status NOT IN ('closed','reaped','suspended')
  AND (excluded.last_seen_at > threads.last_seen_at
       OR excluded.last_seen_at = threads.last_seen_at
          AND (excluded.status IN ('closed','reaped','suspended')
               OR excluded.status = threads.status AND excluded.payload > threads.payload))`, r.ThreadID, r.Agent, r.Status, r.LastSeenAt, r.Payload); err != nil {
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
INSERT INTO threads(thread_id,agent,status,last_seen_at,payload) VALUES(?,?,?,?,?)
ON CONFLICT(thread_id) DO UPDATE SET
 agent=excluded.agent,status=excluded.status,last_seen_at=excluded.last_seen_at,payload=excluded.payload
WHERE threads.status NOT IN ('closed','reaped','suspended')
  AND (excluded.last_seen_at > threads.last_seen_at
       OR excluded.last_seen_at = threads.last_seen_at
          AND (excluded.status IN ('closed','reaped','suspended')
               OR excluded.status = threads.status AND excluded.payload > threads.payload))`,
		r.ThreadID, r.Agent, r.Status, r.LastSeenAt, r.Payload)
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
SET agent=?,status=?,last_seen_at=?,payload=?
WHERE thread_id=? AND status='suspended' AND last_seen_at=?`,
		record.Agent, record.Status, record.LastSeenAt, record.Payload, record.ThreadID, suspendedAt)
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

// DeleteThreadCAS removes only the exact row observed by the pruning read.
// A concurrent heartbeat/status transition changes last_seen/status and makes
// this a safe no-op rather than deleting live truth.
func (s *SQLiteStore) DeleteThreadCAS(threadID, status, lastSeenAt string) (bool, error) {
	result, err := s.db.Exec(`DELETE FROM threads WHERE thread_id=? AND status=? AND last_seen_at=?`, threadID, status, lastSeenAt)
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
	rows, err := s.db.Query(`SELECT thread_id,agent,status,last_seen_at,payload FROM threads ORDER BY thread_id`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: list threads: %w", err)
	}
	defer rows.Close()
	var out []ThreadRecord
	for rows.Next() {
		var r ThreadRecord
		if err := rows.Scan(&r.ThreadID, &r.Agent, &r.Status, &r.LastSeenAt, &r.Payload); err != nil {
			return nil, fmt.Errorf("routerstore: scan thread: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("routerstore: iterate threads: %w", err)
	}
	return out, nil
}
