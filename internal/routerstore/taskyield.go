package routerstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// YieldTaskLease refunds ONLY the current live executable attempt. Explicit
// typed intent, never free-form reason inference. Earlier failures and their
// failure_reason survive. Receipt audit and lease clearing commit together.
// external-wait installs an own dependency only if none exists (or identical);
// it never clears or replaces someone else's established dependency.
func (s *SQLiteStore) YieldTaskLease(agent, taskID, token, disposition, evidenceRef, dependency string) error {
	if disposition != "clean" && disposition != "external-wait" {
		return fmt.Errorf("task yield: disposition must be clean or external-wait")
	}
	evidenceRef = strings.TrimSpace(evidenceRef)
	dependency = strings.TrimSpace(dependency)
	if evidenceRef == "" || len(evidenceRef) > 4096 || len(dependency) > 4096 {
		return fmt.Errorf("task yield: bounded evidence reference required")
	}
	if disposition == "clean" && dependency != "" || disposition == "external-wait" && dependency == "" {
		return fmt.Errorf("task yield: external-wait requires dependency; clean cannot supply one")
	}
	return s.retryWrite(func() error {
		now := s.clock().UTC()
		tx, err := s.beginImmediate()
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		var attempts int
		var thread, blocked string
		err = tx.QueryRow(`SELECT attempts,thread_id,blocked_by FROM tasks WHERE agent=? AND task_id=?`+taskLeaseFence, agent, taskID, token, now.Format(time.RFC3339)).Scan(&attempts, &thread, &blocked)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrLeaseInvalid
		}
		if err != nil {
			return err
		}
		if attempts < 1 {
			return fmt.Errorf("task yield: current claim has no refundable attempt")
		}
		observedBlocked := blocked
		if dependency != "" {
			if blocked != "" && blocked != dependency {
				return fmt.Errorf("task yield: existing dependency differs")
			}
			blocked = dependency
		}
		res, err := tx.Exec(`UPDATE tasks SET status='pending',attempts=attempts-1,blocked_by=?,lease_token='',lease_expires='',claimed_by='',thread_id='',updated=? WHERE agent=? AND task_id=?`+taskLeaseFence+` AND blocked_by=?`, blocked, now.Format(time.RFC3339), agent, taskID, token, now.Format(time.RFC3339), observedBlocked)
		if err != nil {
			return err
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return ErrLeaseInvalid
		}
		// No capability goes into audit. Retain exact before/after accounting.
		audit, err := json.Marshal(struct {
			Evidence   string `json:"evidence_ref"`
			Before     int    `json:"attempts_before"`
			After      int    `json:"attempts_after"`
			Dependency string `json:"dependency"`
		}{evidenceRef, attempts, attempts - 1, blocked})
		if err != nil {
			return err
		}
		// Existing public task-lease audit schema.
		_, err = tx.Exec(`INSERT INTO task_lease_log(ts,task_id,agent,op,session_id,thread_id,host,verdict,reason) VALUES(?,?,?,?,?,?,?,?,?)`, now.Format(AudienceTS), taskID, agent, "yield:"+disposition, "", thread, "", "yielded", string(audit))
		if err != nil {
			return err
		}
		return tx.Commit()
	})
}
