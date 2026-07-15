package tui

import (
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"
)

// Ghosts — residuals of uninstalled apps (Ka the Spirit, safe sigil "*").
//
// `sirsi ghosts --json` returns per-app residual groups. The list shows each
// haunted app and its total reclaimable waste; drilling in (enter) reveals the
// residual files by type (Preferences, Caches, HTTP Storages). Cleaning routes
// through the same tier-honest clean the CLI exposes — ghosts are caution-tier
// residuals, so the console is truthful that removing them uses
// `clean --include-caution`.

type ghostsScreen struct {
	state    loadState
	err      error
	report   ghostReport
	home     string
	selected int
	detail   int // >=0 shows the selected app's residual breakdown

	cleaning   bool
	cleanProof string
	cleanErr   error
	confirm    bool
}

func newGhostsScreen() *ghostsScreen {
	home, _ := os.UserHomeDir()
	return &ghostsScreen{state: stateIdle, home: home, detail: -1}
}

func (s *ghostsScreen) Name() string     { return "Ghosts" }
func (s *ghostsScreen) Sigil() string    { return "spinner-static" } // Ka spirit "*" safe sigil
func (s *ghostsScreen) Layout() Layout   { return LayoutInspect }
func (s *ghostsScreen) State() loadState { return s.state }

// Busy reports an in-flight operation: the residual scan loading or a
// dispatched clean not yet resolved (quit guard, P2#8).
func (s *ghostsScreen) Busy() bool { return s.state == stateLoading || s.cleaning }

func (s *ghostsScreen) HintIDs() []CommandID {
	return []CommandID{CmdMoveDown, CmdInspect, CmdClean, CmdRefresh, CmdTab}
}

func (s *ghostsScreen) RightMeta() string {
	if s.state != stateReady {
		return ""
	}
	return fmt.Sprintf("%d apps · %s", s.report.GhostCount, s.report.TotalWaste)
}

func (s *ghostsScreen) Load() tea.Cmd {
	s.state = stateLoading
	return func() tea.Msg {
		var r ghostReport
		err := decode("ghosts", &r)
		return ghostsLoaded{report: r, err: err}
	}
}

type ghostsLoaded struct {
	report ghostReport
	err    error
}

func (s *ghostsScreen) Update(msg tea.Msg, caps Capabilities) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case ghostsLoaded:
		if m.err != nil {
			s.state = stateError
			s.err = m.err
			return s, nil
		}
		s.state = stateReady
		s.report = m.report
		s.selected = clampSelection(s.selected, len(s.report.Ghosts))
		return s, nil

	case dispatchDone:
		if m.kind != "clean" {
			return s, nil
		}
		s.cleaning = false
		if m.err != nil {
			s.cleanErr = m.err
			return s, nil
		}
		if rep, ok := m.report.(cleanReport); ok {
			s.cleanProof = cleanProofFrom(rep)
		}
		return s, s.Load()

	case keyMsg:
		return s.handleCmd(m.cmd)
	}
	return s, nil
}

func (s *ghostsScreen) handleCmd(cmd Command) (Screen, tea.Cmd) {
	n := len(s.report.Ghosts)
	if s.confirm {
		switch cmd.ID {
		case CmdInspect:
			s.confirm = false
			s.cleaning = true
			s.cleanErr = nil
			return s, runCmd("clean", func() (any, error) {
				var r cleanReport
				// Ghost residuals are caution-tier; the honest clean scope includes them.
				err := decode("clean", &r, "--include-caution", "--confirm", "--yes")
				return r, err
			})
		case CmdBack, CmdClean:
			s.confirm = false
		}
		return s, nil
	}
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
	case CmdClean:
		if n == 0 {
			return s, nil
		}
		s.confirm = true
	}
	return s, nil
}

func (s *ghostsScreen) View(width, height int, caps Capabilities) []string {
	switch s.state {
	case stateIdle, stateLoading:
		return loadingLines("hunting ghost apps…", caps)
	case stateError:
		return errorLines(s.err, caps)
	}
	if len(s.report.Ghosts) == 0 {
		return emptyLines("no ghost residuals — no uninstalled-app leftovers", caps)
	}
	if s.confirm {
		return s.confirmLines(caps)
	}

	lines := []string{
		"  " + Paint(s.report.Summary, TokBrand, caps),
		"",
	}
	cols := []Column{
		{Title: "APP", Width: 30, Align: AlignLeft},
		{Title: "BUNDLE", Width: 32, Align: AlignLeft},
		{Title: "FILES", Width: 6, Align: AlignRight},
		{Title: "WASTE", Width: 9, Align: AlignRight},
	}
	rows := make([]listRow, 0, len(s.report.Ghosts))
	for i, g := range s.report.Ghosts {
		rows = append(rows, listRow{
			cells: []string{
				g.AppName,
				g.BundleID,
				fmt.Sprintf("%d", g.TotalFiles),
				fmtBytes(g.TotalSizeBytes),
			},
			token:    TokWarn,
			selected: i == s.selected,
		})
	}
	// Tail first, so the table window's line budget is exact (P2#6).
	var tail []string
	if s.detail >= 0 && s.detail < len(s.report.Ghosts) {
		tail = append(tail, s.detailLines(caps)...)
	}

	tail = append(tail, "")
	switch {
	case s.cleaning:
		tail = append(tail, "  "+Paint(Sigil("spinner-static", caps)+" clearing residuals…", TokDim, caps))
	case s.cleanErr != nil:
		tail = append(tail, "  "+Paint("BLOCK clean failed: "+s.cleanErr.Error(), TokDanger, caps))
	case s.cleanProof != "":
		tail = append(tail, "  "+Paint(Sigil("check", caps)+" "+s.cleanProof, TokOK, caps))
	default:
		tail = append(tail, "  "+Paint("enter shows residual files · c clears them (caution tier)", TokDim, caps))
	}

	lines = append(lines, renderTableWindow(cols, rows, caps, false, height-len(lines)-len(tail))...)
	lines = append(lines, tail...)
	return lines
}

func (s *ghostsScreen) detailLines(caps Capabilities) []string {
	g := s.report.Ghosts[s.detail]
	out := []string{
		"",
		"  " + Paint("── "+g.AppName+" residuals ", TokAccent, caps),
	}
	for _, r := range g.Residuals {
		out = append(out, fmt.Sprintf("  %s  %-14s %s",
			Paint(Sigil("bullet", caps), TokDim, caps),
			r.Type,
			fmt.Sprintf("%s · %s", fmtBytes(r.SizeBytes), shortPath(r.Path, s.home))))
	}
	return out
}

func (s *ghostsScreen) confirmLines(caps Capabilities) []string {
	return []string{
		"",
		"  " + Paint("CONFIRM CLEAR RESIDUALS", TokWarn, caps),
		"",
		fmt.Sprintf("  This moves ghost-app residuals (%s across %d apps) to the Trash.",
			s.report.TotalWaste, s.report.GhostCount),
		"  Residuals are caution-tier; cleaning uses `clean --include-caution`.",
		"",
		"  " + Paint("enter", TokBrand, caps) + Paint("  confirm and clear", TokDim, caps),
		"  " + Paint("esc", TokBrand, caps) + Paint("    cancel — nothing is removed", TokDim, caps),
	}
}
