package routerstore

import (
	"errors"
	"fmt"
	"time"
)

var ErrNotComplete = errors.New("routerstore: completion gate not satisfied")

type CompletionReport struct {
	Agent                        string        `json:"agent"`
	Runnable                     RunnableState `json:"runnable"`
	BlockedWork                  int           `json:"blocked_work"`
	DeadLetters                  int           `json:"dead_letters"`
	TerminalWakeFailures         int           `json:"terminal_wake_failures"`
	InvalidSatisfiedRequirements int           `json:"invalid_satisfied_requirements"`
	PostCutoverDoneWithoutProof  int           `json:"post_cutover_done_without_proof"`
	Complete                     bool          `json:"complete"`
}

// VerifyCompletion is the only whole-lane completion gate. It checks the
// canonical three-source predicate plus failure/evidence integrity; callers
// may not derive COMPLETE from zero visible inbox rows alone.
func (s *SQLiteStore) VerifyCompletion(agent string) (CompletionReport, error) {
	runnable, err := s.RunnableFor(agent)
	if err != nil {
		return CompletionReport{}, err
	}
	lane, err := s.ClassifyLane(agent, true, 5*time.Minute)
	if err != nil {
		return CompletionReport{}, err
	}
	report := CompletionReport{Agent: agent, Runnable: runnable, BlockedWork: lane.BlockedWork}
	queries := []struct {
		dst *int
		q   string
	}{
		{&report.DeadLetters, `SELECT COUNT(*) FROM items WHERE to_agent=? AND status='dead_letter';`},
		{&report.TerminalWakeFailures, `SELECT COUNT(*) FROM wake_events WHERE agent=? AND status='terminal_failed';`},
		{&report.InvalidSatisfiedRequirements, `SELECT COUNT(*) FROM requirements WHERE owner=? AND status='satisfied'
			AND (commit_ref='' OR tests_ref='' OR security_ref='' OR design_ref='' OR deployment_ref='' OR production_ref='');`},
		{&report.PostCutoverDoneWithoutProof, `SELECT COUNT(*) FROM tasks WHERE agent=? AND status='done' AND result_ref='' AND ` + postEnforcementTaskPredicate("tasks") + `;`},
	}
	for _, query := range queries {
		if err := s.db.QueryRow(query.q, agent).Scan(query.dst); err != nil {
			return CompletionReport{}, fmt.Errorf("routerstore: completion query: %w", err)
		}
	}
	report.Complete = !runnable.Runnable && report.BlockedWork == 0 && report.DeadLetters == 0 &&
		report.TerminalWakeFailures == 0 && report.InvalidSatisfiedRequirements == 0 &&
		report.PostCutoverDoneWithoutProof == 0
	if !report.Complete {
		return report, fmt.Errorf("%w: %+v", ErrNotComplete, report)
	}
	return report, nil
}
