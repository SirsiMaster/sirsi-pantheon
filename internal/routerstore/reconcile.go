package routerstore

import (
	"fmt"
	"time"
)

type ReconcileReport struct {
	Agent                   string               `json:"agent"`
	ExpiredItemLeases       int                  `json:"expired_item_leases"`
	ExpiredTaskLeases       int                  `json:"expired_task_leases"`
	RepairedNonActiveLeases int                  `json:"repaired_non_active_leases"`
	ExpiredWakeLeases       int                  `json:"expired_wake_leases"`
	RequirementTasksCreated int                  `json:"requirement_tasks_created"`
	WakeEventsCreated       int                  `json:"wake_events_created"`
	FalseDoneRejected       int                  `json:"false_done_rejected"`
	State                   LaneOperationalState `json:"state"`
}

// ReconcileOperationalState mechanically repairs the SQLite half of the
// supervision graph. Thread-registration OS truth is reconciled by the router
// supervisor, which then supplies wakeRoutable to the shared classifier.
func (s *Store) ReconcileOperationalState(agent string, wakeRoutable bool) (ReconcileReport, error) {
	now := s.clock().UTC()
	report := ReconcileReport{Agent: agent}
	tx, err := s.beginImmediate()
	if err != nil {
		return report, err
	}
	defer func() { _ = tx.Rollback() }()

	var before int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM items WHERE to_agent=? AND status IN ('claimed','working') AND lease_expires<>'' AND lease_expires<=?;`, agent, now.Format(time.RFC3339)).Scan(&before); err != nil {
		return report, err
	}
	if err = s.reclaimExpiredForTx(tx, now, agent); err != nil {
		return report, err
	}
	report.ExpiredItemLeases = before

	// Ownership on a non-active task is impossible. Repair it regardless of
	// expiry so doctor --fix can recover rows poisoned by older binaries without
	// requiring direct SQLite edits.
	res, err := tx.Exec(`UPDATE tasks SET
		lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE agent=? AND status<>'in-progress' AND
		(lease_token<>'' OR lease_expires<>'' OR claimed_by<>'' OR thread_id<>'');`,
		now.Format(time.RFC3339), agent)
	if err != nil {
		return report, err
	}
	report.RepairedNonActiveLeases, _ = rowsAffectedInt(res)

	res, err = tx.Exec(`UPDATE tasks SET
		status=CASE WHEN attempts>=? THEN 'blocked' ELSE status END,
		failure_reason=CASE WHEN attempts>=? THEN 'task lease attempts exhausted after expiry without completion' ELSE failure_reason END,
		lease_token='',lease_expires='',claimed_by='',thread_id='',updated=?
		WHERE agent=? AND status='in-progress' AND lease_token<>'' AND lease_expires<>'' AND lease_expires<=?;`,
		MaxRetriesPerItem, MaxRetriesPerItem, now.Format(time.RFC3339), agent, now.Format(time.RFC3339))
	if err != nil {
		return report, err
	}
	report.ExpiredTaskLeases, _ = rowsAffectedInt(res)

	res, err = expireWakeLeasesTx(tx, now, agent)
	if err != nil {
		return report, err
	}
	report.ExpiredWakeLeases, _ = rowsAffectedInt(res)

	// Every unmet traced requirement must have an actionable ledger task. This
	// is the repair edge between canon and execution, not a prompt convention.
	res, err = tx.Exec(`INSERT INTO tasks(agent,task_id,subject,status,phase,responsible_party,blocked_by,created,updated,charter,commissioned_at,commissioned_by,outline,timeline,links,test_state,stage,tokens_consumed,duration_seconds)
		SELECT r.owner,'requirement/'||r.req_id,'Implement '||r.req_id||': '||r.title,'pending','Canonical requirements','self','',?,?,?,?,r.owner,NULL,'[]','[]','untested','spec',0,0
		FROM requirements r
		WHERE r.owner=? AND r.status NOT IN ('satisfied','waived')
		AND NOT EXISTS (SELECT 1 FROM tasks t WHERE t.agent=r.owner AND t.task_id='requirement/'||r.req_id);`,
		now.Format(time.RFC3339), now.Format(time.RFC3339), nil, now.Format(time.RFC3339), agent)
	if err != nil {
		return report, fmt.Errorf("routerstore: reconcile requirement tasks: %w", err)
	}
	report.RequirementTasksCreated, _ = rowsAffectedInt(res)
	// An unaudited registry is itself runnable work. Create one durable wake
	// until a real audit store action records evidence.
	var audited int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM state WHERE key=? AND value<>'';`, requirementAuditPrefix+agent).Scan(&audited); err != nil {
		return report, err
	}
	if audited == 0 {
		_, err = tx.Exec(`INSERT OR IGNORE INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
			VALUES(lower(hex(randomblob(16))),?,?,?,?,'canonical requirement audit required',?,?);`,
			"requirement-audit:"+agent, agent, "requirement_audit", agent, now.Format(time.RFC3339), now.Format(time.RFC3339))
		if err != nil {
			return report, err
		}
	}

	// Backfill a wake for runnable work that predates event triggers or whose
	// acknowledged worker later vanished. A terminal failure deliberately
	// suppresses further automatic delivery until an operator repairs it.
	res, err = tx.Exec(`INSERT INTO wake_events(event_id,event_key,agent,source_kind,source_id,reason,created,updated)
		SELECT lower(hex(randomblob(16))),'reconcile:runnable:'||?||':'||?,?,'lane',?,'runnable lane lacks a live wake delivery',?,?
		WHERE (
			EXISTS (SELECT 1 FROM items i WHERE i.to_agent=? AND i.status='open' AND `+actionableItemDependency("i")+`) OR
			EXISTS (SELECT 1 FROM tasks t WHERE t.agent=? AND `+claimableTaskPredicate("t")+`) OR
			EXISTS (SELECT 1 FROM requirements r WHERE r.owner=? AND r.status NOT IN ('satisfied','waived')) OR ?=0
		) AND NOT EXISTS (SELECT 1 FROM wake_events w WHERE w.agent=? AND w.status IN ('pending','leased','terminal_failed'));`,
		agent, now.Format(time.RFC3339Nano), agent, agent, now.Format(time.RFC3339), now.Format(time.RFC3339),
		agent, agent, agent, audited, agent)
	if err != nil {
		return report, fmt.Errorf("routerstore: reconcile runnable wake: %w", err)
	}
	report.WakeEventsCreated, _ = rowsAffectedInt(res)

	// Reject post-cutover done claims that bypassed the evidence-backed leased
	// completion path, and requirement tasks whose requirement remains unmet.
	res, err = tx.Exec(`UPDATE tasks SET status='in-progress',stage=CASE WHEN stage='shipped' THEN 'verify' ELSE stage END,
		blocked_by='',updated=? WHERE agent=? AND status='done' AND `+postEnforcementTaskPredicate("tasks")+`
		AND (result_ref='' OR (task_id LIKE 'requirement/REQ-%' AND EXISTS (
			SELECT 1 FROM requirements r WHERE 'requirement/'||r.req_id=tasks.task_id AND r.status NOT IN ('satisfied','waived')
		)));`, now.Format(time.RFC3339), agent)
	if err != nil {
		return report, fmt.Errorf("routerstore: reject false done: %w", err)
	}
	report.FalseDoneRejected, _ = rowsAffectedInt(res)

	if err = tx.Commit(); err != nil {
		return report, err
	}
	report.State, err = s.ClassifyLane(agent, wakeRoutable, 5*time.Minute)
	return report, err
}

func rowsAffectedInt(result interface{ RowsAffected() (int64, error) }) (int, error) {
	n, err := result.RowsAffected()
	return int(n), err
}
