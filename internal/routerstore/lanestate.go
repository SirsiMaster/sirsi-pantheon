package routerstore

import (
	"fmt"
	"strings"
	"time"
)

const (
	LaneWorking      = "WORKING"
	LaneAssigned     = "ASSIGNED"
	LaneIdleWithWork = "IDLE_WITH_WORK"
	LaneBlocked      = "BLOCKED"
	LaneUnroutable   = "UNROUTABLE"
	LaneComplete     = "COMPLETE"
)

type LaneOperationalState struct {
	Agent                       string        `json:"agent"`
	Classification              string        `json:"classification"`
	Runnable                    RunnableState `json:"runnable"`
	LiveLeases                  int           `json:"live_leases"`
	RecentMutation              bool          `json:"recent_task_record_mutation"`
	BlockedWork                 int           `json:"blocked_work"`
	WakeRoutable                bool          `json:"wake_routable"`
	UnresolvedWakeFailures      int           `json:"unresolved_wake_failures"`
	TerminalWakeFailures        int           `json:"terminal_wake_failures"`
	CompletionIntegrityFailures int           `json:"completion_integrity_failures"`
}

// ClassifyLane is the sole operational-state classifier for Horus and the
// reconciler. wakeRoutable is supplied by the runtime wake registry; process
// existence and thread heartbeat are intentionally not accepted as inputs.
func (s *SQLiteStore) ClassifyLane(agent string, wakeRoutable bool, recentWithin time.Duration) (LaneOperationalState, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return LaneOperationalState{}, fmt.Errorf("routerstore: lane agent is required")
	}
	if recentWithin <= 0 {
		recentWithin = 5 * time.Minute
	}
	now := s.clock().UTC()
	runnable, err := s.RunnableFor(agent)
	if err != nil {
		return LaneOperationalState{}, err
	}
	out := LaneOperationalState{Agent: agent, Runnable: runnable, WakeRoutable: wakeRoutable}

	cutoff := now.Add(-recentWithin).Format(time.RFC3339)
	var workingItems, assignedItems, recentWorkingItems, liveTaskLeases, recentTaskLeases int
	if err = s.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN status='working' AND lease_token<>'' AND lease_expires>? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='claimed' AND lease_token<>'' AND lease_expires>? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='working' AND lease_token<>'' AND lease_expires>? AND lease_updated>=? THEN 1 ELSE 0 END),0)
		FROM items WHERE to_agent=?;`, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339), cutoff, agent).
		Scan(&workingItems, &assignedItems, &recentWorkingItems); err != nil {
		return LaneOperationalState{}, fmt.Errorf("routerstore: classify item leases: %w", err)
	}
	if err = s.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN lease_token<>'' AND lease_expires>? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN lease_token<>'' AND lease_expires>? AND updated>=? THEN 1 ELSE 0 END),0)
		FROM tasks WHERE agent=? AND status='in-progress';`, now.Format(time.RFC3339), now.Format(time.RFC3339), cutoff, agent).
		Scan(&liveTaskLeases, &recentTaskLeases); err != nil {
		return LaneOperationalState{}, fmt.Errorf("routerstore: classify task leases: %w", err)
	}
	out.LiveLeases = workingItems + assignedItems + liveTaskLeases
	out.RecentMutation = recentWorkingItems > 0 || recentTaskLeases > 0

	if err = s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM items i WHERE i.to_agent=? AND (i.status='blocked' OR (i.status='open' AND NOT `+actionableItemDependency("i")+`))) +
		(SELECT COUNT(*) FROM tasks t WHERE t.agent=? AND (t.status='blocked' OR (t.status IN ('pending','in-progress') AND NOT `+actionableTaskDependency("t")+`)));`, agent, agent).
		Scan(&out.BlockedWork); err != nil {
		return LaneOperationalState{}, fmt.Errorf("routerstore: classify blockers: %w", err)
	}
	if out.TerminalWakeFailures, out.UnresolvedWakeFailures, err = s.laneWakeFailures(agent); err != nil {
		return LaneOperationalState{}, err
	}
	if err = s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM items WHERE to_agent=? AND status='dead_letter') +
		(SELECT COUNT(*) FROM requirements WHERE owner=? AND status='satisfied' AND (commit_ref='' OR tests_ref='' OR security_ref='' OR design_ref='' OR deployment_ref='' OR production_ref='')) +
		(SELECT COUNT(*) FROM tasks WHERE agent=? AND status='done' AND result_ref='' AND `+postEnforcementTaskPredicate("tasks")+`)`,
		agent, agent, agent).Scan(&out.CompletionIntegrityFailures); err != nil {
		return LaneOperationalState{}, fmt.Errorf("routerstore: classify completion integrity: %w", err)
	}

	switch {
	case out.RecentMutation:
		out.Classification = LaneWorking
	case out.LiveLeases > 0:
		out.Classification = LaneAssigned
	case out.CompletionIntegrityFailures > 0:
		out.Classification = LaneBlocked
	case runnable.Runnable && (!wakeRoutable || out.UnresolvedWakeFailures > 0):
		out.Classification = LaneUnroutable
	case runnable.Runnable:
		out.Classification = LaneIdleWithWork
	case out.UnresolvedWakeFailures > 0 || out.CompletionIntegrityFailures > 0:
		out.Classification = LaneBlocked
	case out.BlockedWork > 0:
		out.Classification = LaneBlocked
	default:
		out.Classification = LaneComplete
	}
	return out, nil
}

// laneWakeFailures preserves every historical failure while requiring
// strictly newer successful durable work to resolve its routing effect. No
// process, heartbeat, clock tick or manual readiness flag is an input.
func (s *SQLiteStore) laneWakeFailures(agent string) (int, int, error) {
	var latest time.Time
	// Item closed is a terminal transition timestamp, not a mutable metadata touch.
	// Empty/default results and unfenced legacy/admin-only rows are not success.
	rows, err := s.db.Query(`SELECT closed FROM items WHERE to_agent=? AND status='completed'
 AND attempts>0 AND TRIM(result)<>'' AND result<>'(completed without result)'`, agent)
	if err != nil {
		return 0, 0, fmt.Errorf("routerstore: lane item completion evidence: %w", err)
	}
	for rows.Next() {
		var stamp string
		if err = rows.Scan(&stamp); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if t, e := time.Parse(time.RFC3339Nano, stamp); e == nil && t.After(latest) {
			latest = t
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	// A task's updated can be changed after completion. Require its acknowledged
	// fenced claim as well as done/result; the earlier timestamp is a conservative
	// lower bound on that successful run. Metadata edits cannot move this bound.
	rows, err = s.db.Query(`SELECT t.updated,w.updated FROM tasks t JOIN wake_events w
 ON w.agent=t.agent AND w.source_kind='ledger_task' AND w.source_id=t.task_id
 WHERE t.agent=? AND t.status='done' AND t.attempts>0 AND TRIM(t.result_ref)<>''
 AND w.status='acked' AND substr(w.ack_ref,1,length('task-lease:'||t.agent||':'||t.task_id||':'))='task-lease:'||t.agent||':'||t.task_id||':'
 AND length(w.ack_ref)>length('task-lease:'||t.agent||':'||t.task_id||':')`, agent)
	if err != nil {
		return 0, 0, fmt.Errorf("routerstore: lane task completion evidence: %w", err)
	}
	for rows.Next() {
		var done, claim string
		if err = rows.Scan(&done, &claim); err != nil {
			rows.Close()
			return 0, 0, err
		}
		d, de := time.Parse(time.RFC3339Nano, done)
		c, ce := time.Parse(time.RFC3339Nano, claim)
		if de != nil || ce != nil {
			continue
		}
		if c.Before(d) {
			d = c
		}
		if d.After(latest) {
			latest = d
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, 0, err
	}
	rows, err = s.db.Query(`SELECT updated FROM wake_events WHERE agent=? AND status='terminal_failed'`, agent)
	if err != nil {
		return 0, 0, err
	}
	defer rows.Close()
	total, unresolved := 0, 0
	for rows.Next() {
		var stamp string
		if err = rows.Scan(&stamp); err != nil {
			return 0, 0, err
		}
		total++
		failed, e := time.Parse(time.RFC3339Nano, stamp)
		if e != nil || !latest.After(failed) {
			unresolved++
		}
	}
	return total, unresolved, rows.Err()
}
