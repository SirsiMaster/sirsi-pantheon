package main

import "github.com/SirsiMaster/sirsi-pantheon/internal/maat"

// maatTestJournal is intentionally small: command tests use it to pin what a
// command asks the shared journal to append without opening a user journal.
type maatTestJournal struct{ decisions []maat.Decision }

func (j *maatTestJournal) Append(decision maat.Decision) error {
	j.decisions = append(j.decisions, decision)
	return nil
}

func (j *maatTestJournal) Recent(limit int) ([]maat.Decision, error) {
	if limit <= 0 || limit > len(j.decisions) {
		limit = len(j.decisions)
	}
	return append([]maat.Decision(nil), j.decisions[len(j.decisions)-limit:]...), nil
}
