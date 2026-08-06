package routerstore

// Authoritative three-source work predicate — ADR-057 step 3.
//
// A lane may park only when this result is false. Session/process liveness is
// deliberately absent: a heartbeat says that a worker exists, never that work
// is absent or being executed.

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// These expressions are the single executable definition of dependency
// actionability. Every runnable, claim, reconcile, and classification query
// composes them rather than retyping a subset.
func actionableItemDependency(alias string) string {
	return fmt.Sprintf(`(%[1]s.blocked_by='' OR EXISTS (SELECT 1 FROM items d WHERE d.id=%[1]s.blocked_by AND d.status IN ('closed','completed')))`, alias)
}

func actionableTaskDependency(alias string) string {
	return fmt.Sprintf(`(%[1]s.blocked_by='' OR EXISTS (SELECT 1 FROM tasks d WHERE d.agent=%[1]s.agent AND d.task_id=%[1]s.blocked_by AND d.status='done'))`, alias)
}

const requirementAuditPrefix = "requirements:audit:"

// OperationalAgents returns every lane named by durable nonterminal work.
// Supervisors union this with configured adapters and live thread records so a
// missing registry entry cannot hide a stranded lane.
func (s *Store) OperationalAgents() ([]string, error) {
	rows, err := s.db.Query(`SELECT agent FROM (
		SELECT to_agent AS agent FROM items WHERE status IN ('open','claimed','working','blocked')
		UNION SELECT agent FROM tasks WHERE status<>'done'
		UNION SELECT owner AS agent FROM requirements WHERE status NOT IN ('satisfied','waived')
	) WHERE agent<>'' ORDER BY agent`)
	if err != nil {
		return nil, fmt.Errorf("routerstore: operational agents: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var agent string
		if err := rows.Scan(&agent); err != nil {
			return nil, err
		}
		out = append(out, agent)
	}
	if err := rows.Err(); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return out, nil
}

// RunnableState is the durable explanation for a lane's work predicate.
// Counts are returned instead of a bare boolean so Horus and the reconciler
// can report which store makes the lane runnable without reimplementing the
// predicate (and drifting from it).
type RunnableState struct {
	Agent                  string `json:"agent"`
	OpenRouterItems        int    `json:"open_router_items"`
	ActionableLedgerTasks  int    `json:"actionable_ledger_tasks"`
	UnmetRequirements      int    `json:"unmet_requirements"`
	RequirementAuditNeeded bool   `json:"requirement_audit_needed"`
	Runnable               bool   `json:"runnable"`
}

// MarkRequirementAudit records that canon was enumerated for agent. The audit
// marker is distinct from the requirements themselves: an empty registry may
// mean "audited and empty" or "never audited", and only the former may permit
// COMPLETE/parked state.
func (s *Store) MarkRequirementAudit(agent, evidenceRef string) error {
	agent = strings.TrimSpace(agent)
	evidenceRef = strings.TrimSpace(evidenceRef)
	if agent == "" {
		return fmt.Errorf("routerstore: requirement audit agent is required")
	}
	if evidenceRef == "" {
		return fmt.Errorf("routerstore: requirement audit evidence reference is required")
	}
	value := fmt.Sprintf("audited_at=%s evidence=%s", s.clock().UTC().Format(time.RFC3339), evidenceRef)
	_, err := s.db.Exec(`INSERT INTO state(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value;`, requirementAuditPrefix+agent, value)
	if err != nil {
		return fmt.Errorf("routerstore: mark requirement audit for %s: %w", agent, err)
	}
	return nil
}

func requirementAuditExistsTx(tx *sql.Tx, agent string) (bool, error) {
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM state WHERE key=? AND value<>'';`, requirementAuditPrefix+agent).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// RunnableFor evaluates router inbox, task ledger, and canonical requirement
// registry in one serialized read transaction. Callers MUST consume this API;
// duplicating any subset of its SQL would recreate the contradictory surfaces
// ADR-057 exists to eliminate.
func (s *Store) RunnableFor(agent string) (RunnableState, error) {
	agent = strings.TrimSpace(agent)
	if agent == "" {
		return RunnableState{}, fmt.Errorf("routerstore: runnable agent is required")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return RunnableState{}, fmt.Errorf("routerstore: runnable begin: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	state := RunnableState{Agent: agent}
	// An item with a dependency is actionable only after that dependency has a
	// terminal disposition. A missing dependency fails closed as non-actionable.
	if err = tx.QueryRow(`SELECT COUNT(*) FROM items i WHERE i.to_agent=? AND i.status='open' AND `+actionableItemDependency("i")+`;`, agent).Scan(&state.OpenRouterItems); err != nil {
		return RunnableState{}, fmt.Errorf("routerstore: runnable router items: %w", err)
	}

	if err = tx.QueryRow(`SELECT COUNT(*) FROM tasks t
		WHERE t.agent=? AND t.status IN ('pending','in-progress') AND `+actionableTaskDependency("t")+`;`, agent).
		Scan(&state.ActionableLedgerTasks); err != nil {
		return RunnableState{}, fmt.Errorf("routerstore: runnable ledger tasks: %w", err)
	}

	if err = tx.QueryRow(`SELECT COUNT(*) FROM requirements
		WHERE owner=? AND status NOT IN ('satisfied','waived');`, agent).
		Scan(&state.UnmetRequirements); err != nil {
		return RunnableState{}, fmt.Errorf("routerstore: runnable requirements: %w", err)
	}
	audited, err := requirementAuditExistsTx(tx, agent)
	if err != nil {
		return RunnableState{}, fmt.Errorf("routerstore: runnable requirement audit: %w", err)
	}
	state.RequirementAuditNeeded = !audited
	state.Runnable = state.OpenRouterItems > 0 || state.ActionableLedgerTasks > 0 ||
		state.UnmetRequirements > 0 || state.RequirementAuditNeeded
	if err := tx.Commit(); err != nil {
		return RunnableState{}, fmt.Errorf("routerstore: runnable commit: %w", err)
	}
	return state, nil
}
