package routerstore

import (
	"fmt"
	"strings"
	"time"
)

// TaskLeaseLogEntry is one task-lease ownership decision as the service saw
// it: a checkTaskOwner verdict or a BindTaskSession outcome. Audience_log
// (ADR-062 20b.3) answers "was this thread registered"; nothing answered
// "who held this task's lease and what did the service decide" until this
// (router-task-lease-diagnostic-missing).
type TaskLeaseLogEntry struct {
	TS        string `json:"ts"`
	TaskID    string `json:"task_id"`
	Agent     string `json:"agent"`
	Op        string `json:"op"` // "claim_check" | "bind"
	SessionID string `json:"session_id"`
	ThreadID  string `json:"thread_id"`
	Host      string `json:"host"`
	Verdict   string `json:"verdict"` // "allowed" | "refused" | "error"
	Reason    string `json:"reason,omitempty"`
}

// RecordTaskLeaseEvent appends one task-lease decision. Server-side only —
// like RecordAudience, a lane never writes this log itself.
func (s *SQLiteStore) RecordTaskLeaseEvent(e TaskLeaseLogEntry) error {
	if e.TS == "" {
		e.TS = s.clock().UTC().Format(AudienceTS)
	} else {
		e.TS = NormalizeAudienceTS(e.TS)
	}
	_, err := s.exec(`INSERT INTO task_lease_log(ts,task_id,agent,op,session_id,thread_id,host,verdict,reason) VALUES(?,?,?,?,?,?,?,?,?)`,
		e.TS, e.TaskID, e.Agent, e.Op, e.SessionID, e.ThreadID, e.Host, e.Verdict, e.Reason)
	if err != nil {
		return fmt.Errorf("routerstore: RecordTaskLeaseEvent: %w", err)
	}
	return nil
}

// TaskLeaseEventsSince returns every task_lease_log row at or after `since`
// (RFC3339), optionally narrowed to one (agent, taskID) pair. Either filter
// empty means "any". Read-only, exempt from the Rule of Ra gate (like
// AudienceSince) — it answers a question, it does not act.
func (s *SQLiteStore) TaskLeaseEventsSince(agent, taskID, since string) ([]TaskLeaseLogEntry, error) {
	t, perr := time.Parse(time.RFC3339Nano, strings.TrimSpace(since))
	if perr != nil {
		return nil, fmt.Errorf("routerstore: TaskLeaseEventsSince: invalid since %q (want RFC3339): %w", since, perr)
	}
	sinceLog := t.UTC().Format(AudienceTS)

	query := `SELECT ts,task_id,agent,op,session_id,thread_id,host,verdict,reason FROM task_lease_log WHERE ts >= ?`
	args := []any{sinceLog}
	if agent != "" {
		query += ` AND agent = ?`
		args = append(args, agent)
	}
	if taskID != "" {
		query += ` AND task_id = ?`
		args = append(args, taskID)
	}
	query += ` ORDER BY ts`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("routerstore: TaskLeaseEventsSince: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := []TaskLeaseLogEntry{}
	for rows.Next() {
		var e TaskLeaseLogEntry
		if scanErr := rows.Scan(&e.TS, &e.TaskID, &e.Agent, &e.Op, &e.SessionID, &e.ThreadID, &e.Host, &e.Verdict, &e.Reason); scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	if rerr := rows.Err(); rerr != nil {
		return nil, rerr
	}
	return out, nil
}
