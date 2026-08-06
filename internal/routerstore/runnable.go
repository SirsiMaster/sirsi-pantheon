package routerstore

import "fmt"

// RunnableState is the single R1 predicate result. The separate counts keep
// every source visible; consumers must never collapse messages, tasks, and
// canon gaps into an interchangeable number.
type RunnableState struct {
	Agent             string `json:"agent"`
	OpenItems         int    `json:"open_items"`
	ActionableTasks   int    `json:"actionable_tasks"`
	UnmetRequirements int    `json:"unmet_requirements"`
	Runnable          bool   `json:"runnable"`
}

// Runnable computes the authoritative three-source work predicate from one
// durable snapshot. A lane may park only when all three counts are zero.
func (s *Store) Runnable(agent string) (RunnableState, error) {
	state := RunnableState{Agent: agent}
	if agent == "" {
		return state, fmt.Errorf("routerstore: Runnable: agent is required")
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE to_agent=? AND status NOT IN ('closed','completed','dead_letter')`, agent).Scan(&state.OpenItems); err != nil {
		return state, fmt.Errorf("routerstore: runnable items: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE agent=? AND status IN ('pending','in-progress') AND blocked_by=''`, agent).Scan(&state.ActionableTasks); err != nil {
		return state, fmt.Errorf("routerstore: runnable tasks: %w", err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM requirements WHERE agent=? AND status IN ('unmet','in-progress')`, agent).Scan(&state.UnmetRequirements); err != nil {
		return state, fmt.Errorf("routerstore: runnable requirements: %w", err)
	}
	state.Runnable = state.OpenItems > 0 || state.ActionableTasks > 0 || state.UnmetRequirements > 0
	return state, nil
}
