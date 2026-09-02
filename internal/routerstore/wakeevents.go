package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const MaxWakeAttempts = 3

type WakeEvent struct {
	EventID      string `json:"event_id"`
	EventKey     string `json:"event_key"`
	Agent        string `json:"agent"`
	SourceKind   string `json:"source_kind"`
	SourceID     string `json:"source_id"`
	Reason       string `json:"reason"`
	Status       string `json:"status"`
	Attempts     int    `json:"attempts"`
	NextAttempt  string `json:"next_attempt"`
	LeaseToken   string `json:"lease_id,omitempty"`
	LeaseExpires string `json:"lease_expires,omitempty"`
	AckRef       string `json:"ack_ref,omitempty"`
	LastError    string `json:"last_error,omitempty"`
	Created      string `json:"created"`
	Updated      string `json:"updated"`
}

const wakeEventSelect = `SELECT event_id,event_key,agent,source_kind,source_id,reason,status,attempts,next_attempt,lease_token,lease_expires,ack_ref,last_error,created,updated FROM wake_events`

func scanWakeEvent(scanner interface{ Scan(...any) error }) (WakeEvent, error) {
	var e WakeEvent
	err := scanner.Scan(&e.EventID, &e.EventKey, &e.Agent, &e.SourceKind, &e.SourceID, &e.Reason,
		&e.Status, &e.Attempts, &e.NextAttempt, &e.LeaseToken, &e.LeaseExpires,
		&e.AckRef, &e.LastError, &e.Created, &e.Updated)
	return e, err
}

// ClaimWakeEvent leases the oldest due event. Claiming increments the bounded
// attempt counter; delivery acknowledgment must later point to a real store
// action through AckWakeEvent.
func (s *SQLiteStore) ClaimWakeEvent(ttl time.Duration) (*WakeEvent, error) {
	return s.claimWakeEventFor("", ttl)
}

// ClaimWakeEventFor is the supervisor form: one lane's event cannot consume
// another lane's delivery lease merely because it sorts first globally.
func (s *SQLiteStore) ClaimWakeEventFor(agent string, ttl time.Duration) (*WakeEvent, error) {
	if strings.TrimSpace(agent) == "" {
		return nil, fmt.Errorf("routerstore: wake-event agent is required")
	}
	return s.claimWakeEventFor(strings.TrimSpace(agent), ttl)
}

func (s *SQLiteStore) claimWakeEventFor(agent string, ttl time.Duration) (*WakeEvent, error) {
	if ttl <= 0 {
		ttl = time.Minute
	}
	now := s.clock().UTC()
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	tx, err := s.beginImmediate()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = expireWakeLeasesTx(tx, now, ""); err != nil {
		return nil, err
	}
	var id string
	query := `SELECT event_id FROM wake_events WHERE status='pending' AND (next_attempt='' OR next_attempt<=?)`
	args := []any{now.Format(time.RFC3339)}
	if agent != "" {
		query += ` AND agent=?`
		args = append(args, agent)
	}
	err = tx.QueryRow(query+` ORDER BY created,event_id LIMIT 1;`, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoWork
	}
	if err != nil {
		return nil, err
	}
	expires := now.Add(ttl).Format(time.RFC3339)
	res, err := tx.Exec(`UPDATE wake_events SET status='leased',attempts=attempts+1,lease_token=?,lease_expires=?,updated=?
		WHERE event_id=? AND status='pending';`, token, expires, now.Format(time.RFC3339), id)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNoWork
	}
	e, err := scanWakeEvent(tx.QueryRow(wakeEventSelect+` WHERE event_id=?;`, id))
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return &e, nil
}

// AckWakeEvent accepts only a non-empty store-action reference (for example a
// router claim ID or task lease ID), never a process heartbeat.
func (s *SQLiteStore) AckWakeEvent(eventID, token, ackRef string) error {
	ackRef = strings.TrimSpace(ackRef)
	if ackRef == "" {
		return fmt.Errorf("routerstore: wake acknowledgment requires a store-action reference")
	}
	var agent, sourceKind, sourceID string
	if err := s.db.QueryRow(`SELECT agent,source_kind,source_id FROM wake_events
		WHERE event_id=? AND status='leased' AND lease_token=?;`, eventID, token).
		Scan(&agent, &sourceKind, &sourceID); errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseInvalid
	} else if err != nil {
		return err
	}
	if err := s.validateWakeAckRef(agent, sourceKind, sourceID, ackRef); err != nil {
		return err
	}
	now := s.clock().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(`UPDATE wake_events SET status='acked',ack_ref=?,lease_token='',lease_expires='',updated=?
		WHERE event_id=? AND status='leased' AND lease_token=? AND lease_expires>?;`, ackRef, now, eventID, token, now)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrLeaseInvalid
	}
	return nil
}

func (s *SQLiteStore) validateWakeAckRef(agent, sourceKind, sourceID, ref string) error {
	parts := strings.Split(ref, ":")
	switch {
	case len(parts) == 3 && parts[0] == "router-lease" && sourceKind == "router_item" && sourceID == parts[1]:
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE id=? AND lease_token=? AND status IN ('claimed','working');`, parts[1], parts[2]).Scan(&n); err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
	case len(parts) == 3 && parts[0] == "router-lease" && sourceKind == "lane" && sourceID == agent:
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE id=? AND to_agent=? AND lease_token=? AND status IN ('claimed','working');`, parts[1], agent, parts[2]).Scan(&n); err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
	case len(parts) == 4 && parts[0] == "task-lease" && parts[1] == agent &&
		((sourceKind == "ledger_task" && sourceID == parts[2]) ||
			(sourceKind == "requirement" && "requirement/"+sourceID == parts[2])):
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE agent=? AND task_id=? AND lease_token=? AND status='in-progress';`, parts[1], parts[2], parts[3]).Scan(&n); err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
	case len(parts) == 4 && parts[0] == "task-lease" && parts[1] == agent && sourceKind == "lane" && sourceID == agent:
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE agent=? AND task_id=? AND lease_token=? AND status='in-progress';`, parts[1], parts[2], parts[3]).Scan(&n); err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
	case len(parts) == 2 && parts[0] == "requirement-audit" && sourceKind == "requirement_audit" && sourceID == parts[1] && agent == parts[1]:
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM state WHERE key=? AND value<>'';`, requirementAuditPrefix+parts[1]).Scan(&n); err != nil {
			return err
		}
		if n == 1 {
			return nil
		}
	}
	return fmt.Errorf("routerstore: wake acknowledgment reference %q does not resolve to a real store action", ref)
}

// expireWakeLeasesTx treats delivery without a real store acknowledgment as a
// failed wake attempt. A successful process invocation is not success: after
// MaxWakeAttempts silent expiries the event terminally fails and must be
// escalated instead of being retried forever.
func expireWakeLeasesTx(tx *sql.Tx, now time.Time, agent string) (sql.Result, error) {
	nowText := now.UTC().Format(time.RFC3339)
	query := `UPDATE wake_events SET
		status=CASE WHEN attempts>=? THEN 'terminal_failed' ELSE 'pending' END,
		next_attempt=CASE WHEN attempts>=? THEN '' ELSE ? END,
		last_error='wake lease expired without a store acknowledgment',
		lease_token='',lease_expires='',updated=?
		WHERE status='leased' AND lease_expires<>'' AND lease_expires<=?`
	args := []any{MaxWakeAttempts, MaxWakeAttempts, nowText, nowText, nowText}
	if strings.TrimSpace(agent) != "" {
		query += ` AND agent=?`
		args = append(args, strings.TrimSpace(agent))
	}
	return tx.Exec(query+`;`, args...)
}

// FailWakeEvent schedules bounded exponential retry. The third failure is a
// durable terminal escalation state for reconciliation/Horus.
func (s *SQLiteStore) FailWakeEvent(eventID, token, failure string) error {
	now := s.clock().UTC()
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	err = tx.QueryRow(`SELECT attempts FROM wake_events WHERE event_id=? AND status='leased' AND lease_token=? AND lease_expires>?;`,
		eventID, token, now.Format(time.RFC3339)).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseInvalid
	}
	if err != nil {
		return err
	}
	status, next := "pending", now.Add(time.Duration(1<<uint(attempts-1))*time.Minute).Format(time.RFC3339)
	if attempts >= MaxWakeAttempts {
		status, next = "terminal_failed", ""
	}
	_, err = tx.Exec(`UPDATE wake_events SET status=?,next_attempt=?,last_error=?,lease_token='',lease_expires='',updated=? WHERE event_id=?;`,
		status, next, strings.TrimSpace(failure), now.Format(time.RFC3339), eventID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListWakeEvents(agent string) ([]WakeEvent, error) {
	query := wakeEventSelect
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(agent) == "" {
		rows, err = s.db.Query(query + ` ORDER BY created,event_id;`)
	} else {
		rows, err = s.db.Query(query+` WHERE agent=? ORDER BY created,event_id;`, agent)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WakeEvent
	for rows.Next() {
		e, scanErr := scanWakeEvent(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
