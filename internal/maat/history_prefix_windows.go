//go:build windows

package maat

import "fmt"

// The strict retained-parent implementation currently relies on Unix
// descriptor-relative no-follow operations. Fail closed on unsupported hosts.
func (j *FileDecisionJournal) queryHistoryPrefixBound(HistoryQuery) (HistoryPage, error) {
	return HistoryPage{}, fmt.Errorf("maat decision history: strict prefix verification is unavailable on this platform")
}
