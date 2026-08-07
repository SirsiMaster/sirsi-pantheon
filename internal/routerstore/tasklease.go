package routerstore

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// TaskLease is executable ownership of one ledger task. Agent identifies the
// lane; Worker identifies the concrete worker; ThreadID binds the claim to the
// durable conversation/task surface that must acknowledge wake delivery.
type TaskLease struct {
	Agent    string    `json:"agent"`
	TaskID   string    `json:"task_id"`
	Worker   string    `json:"worker"`
	ThreadID string    `json:"thread_id"`
	Token    string    `json:"lease_id"`
	Expires  time.Time `json:"expires"`
	Attempt  int       `json:"attempt"`
}

const taskLeaseFence = ` AND lease_token=? AND lease_expires>? AND status='in-progress'`

// ClaimNextTask atomically reclaims expired claims and leases the oldest
// actionable task. Blocked and done tasks never enter the claim path.
func (s *Store) ClaimNextTask(agent, worker, threadID string, ttl time.Duration) (*TaskLease, error) {
	return s.claimTask(agent, "", worker, threadID, ttl)
}

// ClaimTask atomically leases one exact actionable task. It applies the same
// expiry reconciliation, dependency, retry, contention, and TTL fences as
// ClaimNextTask; taskID changes selection only, never eligibility.
func (s *Store) ClaimTask(agent, taskID, worker, threadID string, ttl time.Duration) (*TaskLease, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return nil, fmt.Errorf("routerstore: exact task claim requires task id")
	}
	return s.claimTask(agent, taskID, worker, threadID, ttl)
}

func (s *Store) claimTask(agent, exactTaskID, worker, threadID string, ttl time.Duration) (*TaskLease, error) {
	agent = strings.TrimSpace(agent)
	worker = strings.TrimSpace(worker)
	threadID = strings.TrimSpace(threadID)
	if agent == "" || worker == "" || threadID == "" {
		return nil, fmt.Errorf("routerstore: task claim requires agent, worker, and thread id")
	}
	var err error
	ttl, err = boundedLeaseTTL(ttl)
	if err != nil {
		return nil, err
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := s.clock().UTC()
	tx, err := s.beginImmediate()
	if err != nil {
		return nil, fmt.Errorf("routerstore: task claim begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Expiry is a failed execution attempt, not an ownership cleanup. Exhausted
	// crash loops are made visibly blocked instead of being reclaimed forever.
	if _, err = tx.Exec(`UPDATE tasks SET
		status=CASE WHEN attempts>=? THEN 'blocked' ELSE status END,
		failure_reason=CASE WHEN attempts>=? THEN 'task lease attempts exhausted after expiry without completion' ELSE failure_reason END,
		lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE agent=? AND lease_token<>'' AND lease_expires<>'' AND lease_expires<=? AND status='in-progress';`,
		MaxRetriesPerItem, MaxRetriesPerItem, now.Format(time.RFC3339), agent, now.Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("routerstore: reclaim task leases: %w", err)
	}

	// Lease poison repair. Only an in-progress row may hold a lease, but that
	// invariant had no enforcer, and the reclaim above cannot fix a violation
	// because its own WHERE clause assumes it holds: a non-active row carrying a
	// lease_token is excluded from reclaim AND from claimableTaskPredicate, so it
	// becomes permanently unclaimable — claim-id returns ErrNoWork while
	// renew/release reject the stale token. Observed in production on schema v15
	// (pantheon-serialized-binary-installer, lease 038b5d77…, expiry
	// 2026-08-06T07:40:25Z, reconciled blocked -> pending with ownership retained).
	//
	// Clearing here rather than shipping an operator repair command means every
	// claim pass self-heals; there is no poisoned state an operator must go into
	// SQLite to fix. Status is deliberately NOT changed — the row keeps whatever
	// disposition reconciliation gave it, so ledger history is not falsified.
	if _, err = tx.Exec(`UPDATE tasks SET
		lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE agent=? AND status<>'in-progress'
		  AND (lease_token<>'' OR lease_expires<>'' OR claimed_by<>'' OR thread_id<>'');`,
		now.Format(time.RFC3339), agent); err != nil {
		return nil, fmt.Errorf("routerstore: clear non-active task leases: %w", err)
	}

	var taskID string
	selectQuery := `SELECT t.task_id FROM tasks t
		WHERE t.agent=? AND ` + claimableTaskPredicate("t") + ` AND t.attempts<?`
	selectArgs := []any{agent, MaxRetriesPerItem}
	if exactTaskID != "" {
		selectQuery += ` AND t.task_id=?`
		selectArgs = append(selectArgs, exactTaskID)
	}
	selectQuery += ` ORDER BY t.created,t.task_id LIMIT 1;`
	err = tx.QueryRow(selectQuery, selectArgs...).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, fmt.Errorf("routerstore: commit task expiry reconciliation: %w", commitErr)
		}
		return nil, ErrNoWork
	}
	if err != nil {
		return nil, fmt.Errorf("routerstore: select task claim: %w", err)
	}
	expires := now.Add(ttl)
	res, err := tx.Exec(`UPDATE tasks AS t SET status='in-progress',claimed_by=?,thread_id=?,lease_token=?,lease_expires=?,attempts=attempts+1,updated=?
		WHERE agent=? AND task_id=? AND `+claimableTaskPredicate("t")+`;`,
		worker, threadID, token, expires.Format(time.RFC3339), now.Format(time.RFC3339), agent, taskID)
	if err != nil {
		return nil, fmt.Errorf("routerstore: claim task: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return nil, ErrNoWork
	}
	var attempt int
	if err = tx.QueryRow(`SELECT attempts FROM tasks WHERE agent=? AND task_id=?;`, agent, taskID).Scan(&attempt); err != nil {
		return nil, fmt.Errorf("routerstore: read task attempt: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("routerstore: task claim commit: %w", err)
	}
	return &TaskLease{Agent: agent, TaskID: taskID, Worker: worker, ThreadID: threadID, Token: token, Expires: expires, Attempt: attempt}, nil
}

func (s *Store) taskLeaseUpdate(agent, taskID, token string, query string, args ...any) error {
	now := s.clock().UTC()
	args = append(args, agent, taskID, token, now.Format(time.RFC3339))
	res, err := s.db.Exec(query+taskLeaseFence+`;`, args...)
	if err != nil {
		return fmt.Errorf("routerstore: task lease update: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrLeaseInvalid
	}
	return nil
}

// RenewTaskLease heartbeats executable ownership. It is a store mutation, so
// supervisors can distinguish work acknowledgment from session liveness.
func (s *Store) RenewTaskLease(agent, taskID, token string, ttl time.Duration) error {
	var err error
	ttl, err = boundedLeaseTTL(ttl)
	if err != nil {
		return err
	}
	now := s.clock().UTC()
	return s.taskLeaseUpdate(agent, taskID, token,
		`UPDATE tasks SET lease_expires=?,updated=? WHERE agent=? AND task_id=?`,
		now.Add(ttl).Format(time.RFC3339), now.Format(time.RFC3339))
}

// CompleteTaskLease records evidence and completes the task under the live
// fence. An empty result reference is refused: completion must point at proof.
func (s *Store) CompleteTaskLease(agent, taskID, token, resultRef string) error {
	resultRef = strings.TrimSpace(resultRef)
	if resultRef == "" {
		return fmt.Errorf("routerstore: task completion evidence reference is required")
	}
	now := s.clock().UTC()
	return s.taskLeaseUpdate(agent, taskID, token,
		`UPDATE tasks SET status='done',result_ref=?,lease_token='',lease_expires='',claimed_by='',thread_id='',updated=? WHERE agent=? AND task_id=?`,
		resultRef, now.Format(time.RFC3339))
}

// ReleaseTaskLease returns a live task to pending after a recoverable failure.
// The attempt counter is retained and the reason is recorded in failure_reason;
// blocked_by remains reserved exclusively for dependency identity.
func (s *Store) ReleaseTaskLease(agent, taskID, token, reason string) error {
	now := s.clock().UTC()
	tx, err := s.beginImmediate()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var attempts int
	err = tx.QueryRow(`SELECT attempts FROM tasks WHERE agent=? AND task_id=?`+taskLeaseFence+`;`,
		agent, taskID, token, now.Format(time.RFC3339)).Scan(&attempts)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrLeaseInvalid
	}
	if err != nil {
		return err
	}
	status, failureReason := "pending", strings.TrimSpace(reason)
	if attempts >= MaxRetriesPerItem {
		status, failureReason = "blocked", "task lease attempts exhausted: "+strings.TrimSpace(reason)
	}
	_, err = tx.Exec(`UPDATE tasks SET status=?,failure_reason=?,lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE agent=? AND task_id=?`+taskLeaseFence+`;`, status, failureReason, now.Format(time.RFC3339), agent, taskID, token, now.Format(time.RFC3339))
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ExpiredLeaseReclaim is what one reclaim pass found and changed.
type ExpiredLeaseReclaim struct {
	Agent        string `json:"agent"`
	TaskID       string `json:"task_id"`
	AttemptsUsed int    `json:"attempts"`
	Blocked      bool   `json:"blocked"`
}

// ReclaimExpiredTaskLeases runs the same expiry reconciliation ClaimTask
// already applies per-agent as a side effect of that agent claiming — but
// across every agent's queue, not just the caller's own. Without this, a
// task orphaned by a session that claimed it and never returned (crashed,
// hit a session boundary, lost its lease token) stays lease-held and
// unclaimable until that exact agent happens to call claim again — every
// OTHER agent's queue self-heals on its own next claim, but a queue nobody
// is actively driving never gets the chance. Non-destructive: it only clears
// lease ownership pointers (lease_token, lease_expires, claimed_by,
// thread_id); status is left untouched except the same attempts-exhausted
// case ClaimTask already handles (flip to blocked, never silently reclaimed
// forever). dryRun reports what WOULD change without writing.
func (s *Store) ReclaimExpiredTaskLeases(dryRun bool) ([]ExpiredLeaseReclaim, error) {
	now := s.clock().UTC()
	rows, err := s.db.Query(`SELECT agent,task_id,attempts FROM tasks
		WHERE lease_token<>'' AND lease_expires<>'' AND lease_expires<=? AND status='in-progress'
		ORDER BY agent,task_id;`, now.Format(time.RFC3339))
	if err != nil {
		return nil, fmt.Errorf("routerstore: find expired task leases: %w", err)
	}
	var found []ExpiredLeaseReclaim
	for rows.Next() {
		var r ExpiredLeaseReclaim
		if err := rows.Scan(&r.Agent, &r.TaskID, &r.AttemptsUsed); err != nil {
			rows.Close()
			return nil, fmt.Errorf("routerstore: scan expired task lease: %w", err)
		}
		r.Blocked = r.AttemptsUsed >= MaxRetriesPerItem
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	if dryRun || len(found) == 0 {
		return found, nil
	}

	tx, err := s.beginImmediate()
	if err != nil {
		return nil, fmt.Errorf("routerstore: reclaim expired task leases begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = tx.Exec(`UPDATE tasks SET
		status=CASE WHEN attempts>=? THEN 'blocked' ELSE status END,
		failure_reason=CASE WHEN attempts>=? THEN 'task lease attempts exhausted after expiry without completion' ELSE failure_reason END,
		lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE lease_token<>'' AND lease_expires<>'' AND lease_expires<=? AND status='in-progress';`,
		MaxRetriesPerItem, MaxRetriesPerItem, now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		return nil, fmt.Errorf("routerstore: reclaim expired task leases: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("routerstore: reclaim expired task leases commit: %w", err)
	}
	return found, nil
}
