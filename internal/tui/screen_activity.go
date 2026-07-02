package tui

import (
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
}

func newActivityScreen() *activityScreen {
	home, _ := os.UserHomeDir()
	return &activityScreen{state: stateIdle, home: home, detail: -1}
}

func (s *activityScreen) Name() string     { return "Activity" }
func (s *activityScreen) Sigil() string    { return "bullet" } // ledger (neutral safe sigil)
func (s *activityScreen) Layout() Layout   { return LayoutSurvey }
func (s *activityScreen) State() loadState { return s.state }

// Busy reports an in-flight ledger load. Activity is read-only — it never
// dispatches an action, so loading is its only busy state (quit guard, P2#8).
func (s *activityScreen) Busy() bool { return s.state == stateLoading }

func (s *activityScreen) HintIDs() []CommandID {
	return []CommandID{CmdMoveDown, CmdInspect, CmdRefresh, CmdTab, CmdQuit}
}

func (s *activityScreen) RightMeta() string {
	if s.state != stateReady {
		return ""
	}
	return fmt.Sprintf("%d operations", s.report.Count)
}

func (s *activityScreen) Load() tea.Cmd {
	s.state = stateLoading
	return func() tea.Msg {
		var r activityReport
		err := decode("activity", &r)
		return activityLoaded{report: r, err: err}
	}
}

type activityLoaded struct {
	report activityReport
	err    error
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

	case keyMsg:
		return s.handleCmd(m.cmd)
	}
	return s, nil
}

func (s *activityScreen) handleCmd(cmd Command) (Screen, tea.Cmd) {
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

func (s *activityScreen) View(width, height int, caps Capabilities) []string {
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
