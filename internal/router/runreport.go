// runreport.go — heal collector + API reachability probe + the owner-facing
// run-report writer (local sovereignty P1, owner directive 2026-07-23). Every
// supervisor pass ends by appending one plain-English Run to
// ~/.sirsi/conduit-report.json (internal/report contract), which `sirsi
// report` and the menubar "Last check" line render. Duties record what they
// fixed via RecordHeal as they go; the pass drains the collector at the end.
package router

import (
	"net/http"
	"sync"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/report"
)

// healCollector gathers owner-visible heal lines across one supervisor pass.
// Package-level because duties are plain functions; drained per pass. The
// supervisor loop is single-flight per process, so contention is nil — the
// mutex is for the -race suite's benefit, not a concurrency design.
var (
	healMu    sync.Mutex
	healLines []string
)

// RecordHeal notes a completed self-repair in plain English. Duties call it
// as they fix things; the current pass's run report carries every line.
func RecordHeal(line string) {
	healMu.Lock()
	defer healMu.Unlock()
	healLines = append(healLines, line)
}

// drainHeals returns and clears the collected heal lines.
func drainHeals() []string {
	healMu.Lock()
	defer healMu.Unlock()
	lines := healLines
	healLines = nil
	return lines
}

// anthropicProbeURL answers ANY HTTP status when the API is reachable —
// reachability is transport-level (a 401 without auth is still "reachable").
const anthropicProbeURL = "https://api.anthropic.com/v1/messages"

// ProbeAnthropicReachable is the production probe: one bounded GET; any HTTP
// response = reachable.
func ProbeAnthropicReachable() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(anthropicProbeURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// Report wiring follows the auto-heal seam pattern (Rule A16/A21): INERT by
// default so library consumers and tests never touch the network or the
// user's home; cmd/sirsi wires the real probe + writer at init. A test that
// wants the write injects its own.
var (
	runReportMu sync.RWMutex
	runReportFn = func(now time.Time, duties []DutyResult, heals []string) {}
)

// SetRunReportFn installs the run-report writer the supervisor calls per pass.
func SetRunReportFn(fn func(now time.Time, duties []DutyResult, heals []string)) {
	runReportMu.Lock()
	defer runReportMu.Unlock()
	if fn != nil {
		runReportFn = fn
	}
}

func getRunReportFn() func(time.Time, []DutyResult, []string) {
	runReportMu.RLock()
	defer runReportMu.RUnlock()
	return runReportFn
}

// WriteSupervisorRun is the REAL writer cmd/sirsi wires in: compose the
// owner-facing Run and append it to the report file.
func WriteSupervisorRun(now time.Time, duties []DutyResult, heals []string) {
	var escalations []string
	for _, d := range duties {
		if d.Error != "" {
			escalations = append(escalations, d.Name+": "+d.Error)
		}
	}
	outcome := report.OutcomeGreen
	switch {
	case len(escalations) > 0:
		outcome = report.OutcomeDegraded
	case len(heals) > 0:
		outcome = report.OutcomeHealed
	}
	reachable := ProbeAnthropicReachable()
	_ = report.Append(report.Path(), report.Run{
		TS:           now.UTC().Format(time.RFC3339),
		Source:       "supervisor",
		Outcome:      outcome,
		Heals:        heals,
		Escalations:  escalations,
		APIReachable: &reachable,
	})
}

// writeRunReport drains this pass's heals and hands them to the wired writer
// (inert unless cmd/sirsi installed WriteSupervisorRun). Outcome grading and
// the append itself live in WriteSupervisorRun. Best-effort by contract: a
// report failure must never fail the supervisor pass.
func writeRunReport(now time.Time, duties []DutyResult) {
	getRunReportFn()(now, duties, drainHeals())
}
