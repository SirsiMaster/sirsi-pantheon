package tui

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Activity — the provenance ledger from `sirsi activity --json`.
//
// A read-only record of what sirsi actually changed: each entry is an action
// (clean, purge), its target path, and the bytes affected, read from the
// operations log. Drilling in (enter) shows the full target path and timestamp.
// This screen is the audit surface — it never dispatches a destructive action.

type activityScreen struct {
	state    loadState
	err      error
	report   activityReport
	home     string
	selected int
	detail   int

	// Ma'at is a second, read-only view inside Activity, not a sixth console
	// screen. This preserves the operator console's five-screen information
	// architecture while giving every operator the same System One casebook that
	// the CLI, MCP, Horus dashboard, and native app project.
	showMaat                                     bool
	maatState                                    loadState
	maatErr                                      error
	maatReport                                   maatCasebookReport
	maatSelected                                 int
	maatDetail                                   int
	maatGeneration                               uint64
	maatCancel                                   context.CancelFunc
	maatQuery, maatScope, maatStatus, maatCursor string
	maatEditing                                  bool
	maatDraft                                    string
}

func newActivityScreen() *activityScreen {
	home, _ := os.UserHomeDir()
	// Both drill-downs begin closed. Leaving maatDetail at Go's zero value makes
	// the first selected case look open, so Enter hides it instead of showing the
	// case's prescribed resolution.
	return &activityScreen{state: stateIdle, home: home, detail: -1, maatDetail: -1}
}

func (s *activityScreen) Name() string     { return "Activity" }
func (s *activityScreen) Sigil() string    { return "bullet" } // ledger (neutral safe sigil)
func (s *activityScreen) Layout() Layout   { return LayoutSurvey }
func (s *activityScreen) State() loadState { return s.state }

// Busy reports an in-flight evidence read. Activity is read-only — it never
// dispatches an action, so loading is its only busy state (quit guard, P2#8).
func (s *activityScreen) Busy() bool {
	return s.state == stateLoading || s.maatState == stateLoading
}

func (s *activityScreen) HintIDs() []CommandID {
	if s.showMaat {
		return []CommandID{CmdInspect, CmdMaatContinue, CmdMaatSearch, CmdMaatScope, CmdMaatStatus, CmdRefresh, CmdMaatCasebook, CmdTab, CmdQuit}
	}
	return []CommandID{CmdMoveDown, CmdInspect, CmdMaatCasebook, CmdRefresh, CmdTab, CmdQuit}
}

func (s *activityScreen) RightMeta() string {
	if s.showMaat && s.maatState == stateReady {
		return fmt.Sprintf("page: %d open · %d urgent", s.maatReport.Summary.Open, s.maatReport.Summary.Urgent)
	}
	if s.state != stateReady {
		return ""
	}
	return fmt.Sprintf("%d operations", s.report.Count)
}

func (s *activityScreen) Load() tea.Cmd {
	s.state = stateLoading
	return tea.Batch(func() tea.Msg {
		var r activityReport
		err := decode("activity", &r)
		return activityLoaded{report: r, err: err}
	}, s.loadMaat(""))
}

type activityLoaded struct {
	report activityReport
	err    error
}

type maatCasebookLoaded struct {
	generation                uint64
	cursor, snapshot, queryID string
	report                    maatCasebookReport
	err                       error
}

func (s *activityScreen) Update(msg tea.Msg, caps Capabilities) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case activityLoaded:
		if m.err != nil {
			s.state = stateError
			s.err = m.err
			return s, nil
		}
		s.state = stateReady
		s.report = m.report
		s.selected = clampSelection(s.selected, len(s.report.Entries))
		return s, nil

	case maatCasebookLoaded:
		if m.generation != s.maatGeneration {
			return s, nil
		}
		if m.err == nil && m.cursor != "" && (m.snapshot != m.report.History.Snapshot || m.queryID != m.report.History.QueryID) {
			m.err = fmt.Errorf("history snapshot changed; u restarts the query")
		}
		if m.err != nil {
			s.maatState = stateError
			s.maatErr = m.err
			return s, nil
		}
		s.maatState = stateReady
		s.maatErr = nil
		s.maatDetail = -1
		s.maatReport = m.report
		s.maatSelected = clampSelection(s.maatSelected, len(s.maatReport.Cases))
		return s, nil

	case keyMsg:
		return s.handleCmd(m.cmd)
	}
	return s, nil
}

func (s *activityScreen) handleCmd(cmd Command) (Screen, tea.Cmd) {
	if cmd.ID == CmdMaatCasebook {
		s.showMaat = !s.showMaat
		if !s.showMaat {
			s.CancelRead()
		} else if s.maatState == stateIdle {
			return s, s.loadMaat("")
		}
		return s, nil
	}
	if s.showMaat {
		return s.handleMaatCmd(cmd)
	}
	n := len(s.report.Entries)
	switch cmd.ID {
	case CmdMoveDown:
		s.selected = clampSelection(s.selected+1, n)
	case CmdMoveUp:
		s.selected = clampSelection(s.selected-1, n)
	case CmdTop:
		s.selected = 0
	case CmdBottom:
		s.selected = clampSelection(n-1, n)
	case CmdInspect:
		if s.detail == s.selected {
			s.detail = -1
		} else {
			s.detail = s.selected
		}
	case CmdBack:
		s.detail = -1
	case CmdRefresh:
		return s, s.Load()
	}
	return s, nil
}

func (s *activityScreen) handleMaatCmd(cmd Command) (Screen, tea.Cmd) {
	switch cmd.ID {
	case CmdMaatContinue:
		if s.maatState == stateError {
			return s, s.loadMaat(s.maatCursor)
		}
		if s.maatState != stateLoading && s.maatReport.History.NextCursor != "" {
			return s, s.loadMaat(s.maatReport.History.NextCursor)
		}
		return s, nil
	case CmdMaatSearch:
		s.CancelRead()
		s.maatEditing = true
		s.maatDraft = s.maatQuery
		return s, nil
	case CmdMaatScope:
		if s.maatScope == "system-one" {
			s.maatScope = "all"
		} else {
			s.maatScope = "system-one"
		}
		return s, s.loadMaat("")
	case CmdMaatStatus:
		switch s.maatStatus {
		case "":
			s.maatStatus = "open"
		case "open":
			s.maatStatus = "resolved"
		default:
			s.maatStatus = ""
		}
		return s, s.loadMaat("")
	case CmdRefresh:
		return s, s.loadMaat("")
	case CmdBack:
		s.CancelRead()
		s.maatDetail = -1
		return s, nil
	}
	n := len(s.maatReport.Cases)
	switch cmd.ID {
	case CmdMoveDown:
		s.maatSelected = clampSelection(s.maatSelected+1, n)
	case CmdMoveUp:
		s.maatSelected = clampSelection(s.maatSelected-1, n)
	case CmdTop:
		s.maatSelected = 0
	case CmdBottom:
		s.maatSelected = clampSelection(n-1, n)
	case CmdInspect:
		if s.maatDetail == s.maatSelected {
			s.maatDetail = -1
		} else {
			s.maatDetail = s.maatSelected
		}
	case CmdBack:
		s.maatDetail = -1
	case CmdRefresh:
		return s, s.Load()
	}
	return s, nil
}

func (s *activityScreen) View(width, height int, caps Capabilities) []string {
	if s.showMaat {
		return s.maatView(height, caps)
	}
	switch s.state {
	case stateIdle, stateLoading:
		return loadingLines("reading operations ledger…", caps)
	case stateError:
		return errorLines(s.err, caps)
	}
	if len(s.report.Entries) == 0 {
		return emptyLines("no operations logged yet — nothing has been changed", caps)
	}

	lines := []string{
		"  " + Paint("what sirsi actually changed", TokBrand, caps),
		"",
	}
	cols := []Column{
		{Title: "WHEN", Width: 12, Align: AlignLeft},
		{Title: "ACTION", Width: 8, Align: AlignLeft},
		{Title: "TARGET", Width: 44, Align: AlignLeft},
		{Title: "SIZE", Width: 9, Align: AlignRight},
	}
	rows := make([]listRow, 0, len(s.report.Entries))
	for i, e := range s.report.Entries {
		size := "—"
		if e.Bytes > 0 {
			size = fmtBytes(e.Bytes)
		}
		rows = append(rows, listRow{
			cells: []string{
				relTime(e.Time),
				e.Action,
				shortPath(e.Target, s.home),
				size,
			},
			token:    TokDim,
			selected: i == s.selected,
		})
	}
	// Tail first, so the table window's line budget is exact (P2#6).
	var tail []string
	if s.detail >= 0 && s.detail < len(s.report.Entries) {
		e := s.report.Entries[s.detail]
		tail = append(tail,
			"",
			"  "+Paint("── "+e.Action+" ", TokAccent, caps),
			"  "+Paint("when:   ", TokDim, caps)+e.Time,
			"  "+Paint("target: ", TokDim, caps)+e.Target,
			"  "+Paint("bytes:  ", TokDim, caps)+fmt.Sprintf("%d", e.Bytes),
			"  "+Paint("source: ", TokDim, caps)+e.Source,
		)
	}
	// u is the update/refresh key (P1#2 rebinding: r = relieve, u = update).
	tail = append(tail, "", "  "+Paint("read-only audit ledger · enter for detail · u update", TokDim, caps))

	lines = append(lines, renderTableWindow(cols, rows, caps, false, height-len(lines)-len(tail))...)
	lines = append(lines, tail...)
	return lines
}

func (s *activityScreen) maatView(height int, caps Capabilities) []string {
	if s.maatEditing {
		return []string{"  Search canonical history (512 bytes maximum):", "  " + s.maatDraft, "  enter search · esc cancel"}
	}
	switch s.maatState {
	case stateIdle:
		return []string{"  Ma’at history read paused. u restarts; / searches; m returns to operations.", "  " + s.maatQueryLabel()}
	case stateLoading:
		return loadingLines("opening Ma'at System One casebook…", caps)
	case stateError:
		return []string{
			"  " + Paint("Ma'at casebook needs attention", TokWarn, caps),
			"",
			"  " + Paint(s.maatErr.Error(), TokDim, caps),
			"",
			"  " + Paint("n retries this read · u restarts the query · m returns to the operations ledger", TokDim, caps),
		}
	}
	if s.maatReport.History.Coverage != "complete" {
		return []string{"  " + s.maatQueryLabel(), "  Ma’at history: " + s.maatReport.History.Coverage, "  No provisional cases. n continues a bounded scan; u restarts; / searches."}
	}
	if len(s.maatReport.Cases) == 0 {
		return []string{
			"  " + s.maatQueryLabel(),
			"  " + Paint("Ma'at System One casebook", TokBrand, caps),
			"",
			"  " + Paint(s.maatHistoryLabel()+" · no cases on this page; inspect integrity before concluding absence.", TokDim, caps),
			"  " + Paint("m returns to the operations ledger · u refreshes the evidence journal", TokDim, caps),
		}
	}

	lines := []string{
		"  " + s.maatHistoryLabel(),
		"  " + s.maatQueryLabel(),
		"  " + Paint("Ma'at System One · evidence-bound casebook", TokBrand, caps),
		"  " + Paint(fmt.Sprintf("page: %d cases · %d open · %d urgent · %d resolved", s.maatReport.Summary.Total, s.maatReport.Summary.Open, s.maatReport.Summary.Urgent, s.maatReport.Summary.Resolved), TokDim, caps),
		"",
	}
	cols := []Column{
		{Title: "PRIORITY", Width: 10, Align: AlignLeft},
		{Title: "STATUS", Width: 10, Align: AlignLeft},
		{Title: "CASE", Width: 16, Align: AlignLeft},
		{Title: "ASSESSMENT", Width: 42, Align: AlignLeft},
	}
	rows := make([]listRow, 0, len(s.maatReport.Cases))
	for i, c := range s.maatReport.Cases {
		assessment := c.Why
		if assessment == "" {
			assessment = c.Determination
		}
		rows = append(rows, listRow{cells: []string{c.Priority, c.Status, c.Category, assessment}, token: maatCaseToken(c), selected: i == s.maatSelected})
	}
	var tail []string
	if s.maatDetail >= 0 && s.maatDetail < len(s.maatReport.Cases) {
		c := s.maatReport.Cases[s.maatDetail]
		tail = append(tail, "", "  "+Paint("── "+c.Category+" ", TokAccent, caps))
		if c.Assessed != "" {
			tail = append(tail, "  "+Paint("assessment: ", TokDim, caps)+c.Assessed)
		}
		tail = append(tail, "  "+Paint("determination: ", TokDim, caps)+c.Determination)
		if c.Evidence != "" {
			tail = append(tail, "  "+Paint("evidence: ", TokDim, caps)+c.Evidence)
		}
		if c.SystemOne != nil {
			tail = append(tail, "  "+Paint("System One: ", TokDim, caps)+fmt.Sprintf("%s · %.0f%% confidence · feather %d/100", c.SystemOne.Gate, c.SystemOne.Confidence*100, c.SystemOne.FeatherWeight))
			modelScope := "external"
			if c.SystemOne.Model.Local {
				modelScope = "local"
			}
			tail = append(tail, "  "+Paint("screen model: ", TokDim, caps)+fmt.Sprintf("%s %s · %s · %dms", c.SystemOne.Model.Provider, c.SystemOne.Model.Version, modelScope, c.SystemOne.Model.LatencyMS))
			floorState := "failed"
			if c.SystemOne.Floor.Passed {
				floorState = "passed"
			}
			tail = append(tail, "  "+Paint("deterministic floor: ", TokDim, caps)+floorState)
			for _, check := range c.SystemOne.Floor.Checks {
				state := "failed"
				if check.Passed {
					state = "passed"
				}
				line := "  " + Paint("floor check ["+check.Name+"]: ", TokDim, caps) + state
				if check.Detail != "" {
					line += " · " + check.Detail
				}
				tail = append(tail, line)
			}
			if c.SystemOne.Escalation != nil && c.SystemOne.Escalation.Reason != "" {
				tail = append(tail, "  "+Paint("required review: ", TokDim, caps)+c.SystemOne.Escalation.Reason)
			}
			for _, finding := range c.SystemOne.Findings {
				heading := finding.Severity + " · " + finding.Category
				tail = append(tail, "  "+Paint("finding ("+heading+"): ", TokDim, caps)+finding.Claim)
				if finding.FixHint != "" {
					tail = append(tail, "  "+Paint("prescribed next step: ", TokAccent, caps)+finding.FixHint)
				}
			}
		}
		if c.NextAction != nil {
			tail = append(tail, "  "+Paint("next step: ", TokDim, caps)+c.NextAction.Title)
			tail = append(tail, "  "+Paint(c.NextAction.Detail, TokDim, caps))
			for _, step := range c.NextAction.Steps {
				tail = append(tail, "  "+Paint(fmt.Sprintf("recovery level %d · %s: ", step.Level, step.Title), TokAccent, caps)+step.Detail)
			}
			if c.NextAction.RequiresConfirmation {
				tail = append(tail, "  "+Paint("requires explicit owner confirmation · native Pantheon Casebook can record it", TokWarn, caps))
			}
		}
		for _, link := range []struct{ label, value string }{{"verified repair", c.VerifiedRepairEvidence}, {"repair outcome", c.RepairOutcomeEvidence}, {"pending repair attempt", c.PendingRepairAttemptEvidence}} {
			if link.value != "" {
				tail = append(tail, "  "+link.label+": "+link.value)
			}
		}
		if c.SystemOneCalibration != nil {
			tail = append(tail, "  independent calibration: "+c.SystemOneCalibration.ScreenEvidence+" → "+c.SystemOneCalibration.FrontierEvidence)
		}
		if c.Resolution != "" {
			tail = append(tail, "  "+Paint("owner acceptance: ", TokDim, caps)+c.Resolution)
		}
	}
	tail = append(tail, "", "  "+Paint("m operations · n older/continue/retry · u restart", TokDim, caps), "  "+Paint("enter detail · / search · o scope · v status · read-only", TokDim, caps))
	lines = append(lines, renderTableWindow(cols, rows, caps, false, height-len(lines)-len(tail))...)
	return append(lines, tail...)
}

func maatCaseToken(c maatCase) Token {
	switch c.Priority {
	case "urgent":
		return TokDanger
	case "high":
		return TokWarn
	default:
		return TokDim
	}
}

// relTime renders the entry's timestamp as a compact relative label. The oplog
// time is "2006-01-02T15:04:05" (local, no zone); an unparseable value is shown
// verbatim (never dropped).
func relTime(ts string) string {
	t, err := time.ParseInLocation("2006-01-02T15:04:05", ts, time.Local)
	if err != nil {
		if len(ts) > 12 {
			return ts[:12]
		}
		return ts
	}
	d := time.Since(t)
	if d < 0 {
		d = 0
	}
	return agoLabel(int64(d.Seconds()))
}
