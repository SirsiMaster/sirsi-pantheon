package main

import "github.com/SirsiMaster/sirsi-pantheon/internal/maat"

// maatTestJournal is intentionally small: command tests use it to pin what a
// command asks the shared journal to append without opening a user journal.
type maatTestJournal struct{ decisions []maat.Decision }

func (j *maatTestJournal) Append(decision maat.Decision) error {
	j.decisions = append(j.decisions, decision)
	return nil
}

func (j *maatTestJournal) AppendOnce(decision maat.Decision) (bool, error) {
	for _, existing := range j.decisions {
		if existing.Kind == decision.Kind && existing.Evidence == decision.Evidence {
			return false, nil
		}
	}
	j.decisions = append(j.decisions, decision)
	return true, nil
}

func (j *maatTestJournal) Recent(limit int) ([]maat.Decision, error) {
	if limit <= 0 || limit > len(j.decisions) {
		limit = len(j.decisions)
	}
	return append([]maat.Decision(nil), j.decisions[len(j.decisions)-limit:]...), nil
}

func (j *maatTestJournal) FindUniqueByEvidence(evidence string) (maat.Decision, int, error) {
	var found maat.Decision
	matches := 0
	for _, decision := range j.decisions {
		if decision.Evidence == evidence {
			matches++
			if matches == 1 {
				found = decision
			}
		}
	}
	return found, matches, nil
}
