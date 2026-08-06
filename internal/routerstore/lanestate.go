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
	TerminalWakeFailures        int           `json:"terminal_wake_failures"`
	CompletionIntegrityFailures int           `json:"completion_integrity_failures"`
}

// ClassifyLane is the sole operational-state classifier for Horus and the
// reconciler. wakeRoutable is supplied by the runtime wake registry; process
// existence and thread heartbeat are intentionally not accepted as inputs.
func (s *Store) ClassifyLane(agent string, wakeRoutable bool, recentWithin time.Duration) (LaneOperationalState, error) {
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
	if err = s.db.QueryRow(`SELECT COUNT(*) FROM wake_events WHERE agent=? AND status='terminal_failed'`, agent).Scan(&out.TerminalWakeFailures); err != nil {
		return LaneOperationalState{}, fmt.Errorf("routerstore: classify terminal wakes: %w", err)
	}
	if err = s.db.QueryRow(`SELECT
		(SELECT COUNT(*) FROM items WHERE to_agent=? AND status='dead_letter') +
		(SELECT COUNT(*) FROM requirements WHERE owner=? AND status='satisfied' AND (commit_ref='' OR tests_ref='' OR security_ref='' OR design_ref='' OR deployment_ref='' OR production_ref='')) +
		(SELECT COUNT(*) FROM tasks WHERE agent=? AND status='done' AND result_ref='' AND updated>=(SELECT value FROM state WHERE key='operational_enforcement_since'))`,
		agent, agent, agent).Scan(&out.CompletionIntegrityFailures); err != nil {
		return LaneOperationalState{}, fmt.Errorf("routerstore: classify completion integrity: %w", err)
	}

	switch {
	case out.RecentMutation:
		out.Classification = LaneWorking
	case out.LiveLeases > 0:
		out.Classification = LaneAssigned
	case runnable.Runnable && (!wakeRoutable || out.TerminalWakeFailures > 0):
		out.Classification = LaneUnroutable
	case runnable.Runnable:
		out.Classification = LaneIdleWithWork
	case out.TerminalWakeFailures > 0 || out.CompletionIntegrityFailures > 0:
		out.Classification = LaneBlocked
	case out.BlockedWork > 0:
		out.Classification = LaneBlocked
	default:
		out.Classification = LaneComplete
	}
	return out, nil
}
