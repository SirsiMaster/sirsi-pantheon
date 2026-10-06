//go:build windows

package maat

import "fmt"

// Windows currently has no descriptor-relative no-follow replacement primitive
// in this package. Refuse the mutating repair rather than approximating the
// Unix safety contract with a pathname rewrite.
func repairInvalidRecords(*FileDecisionJournal) (JournalRepairReceipt, error) {
	return JournalRepairReceipt{}, fmt.Errorf("maat decision journal: preservation repair is unavailable on this platform; no journal data changed")
}

func withJournalMutationLock(_ string, fn func() error) error {
	return fn()
}
