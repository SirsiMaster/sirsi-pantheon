package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Waste — scan → review → clean, with a freed-space proof (menubar #130 flow).
//
// `sirsi scan --json` produces the per-item review list. Each item shows its
// rule, size, and severity; the operator toggles which severity TIERS to include
// (space), drills into a finding (enter) to see its full path, then cleans (c).
//
// Honest dispatch: the merged CLI contract (PR #139) cleans by TIER, not by an
// arbitrary path subset — `sirsi clean --confirm --yes` targets the safe tier and
// `--include-caution` adds the caution tier. There is no `--only <paths>` flag,
// so the console does NOT pretend to clean a hand-picked subset; the toggles
// choose the tier(s) and the freed-space proof comes straight from the clean
// report. Protected items are shown but never cleanable (danger/BLOCK).

type wasteScreen struct {
	state    loadState
	err      error
	report   scanReport
	home     string
	selected int

	// includeCaution mirrors the real CLI scope switch; toggled per selected row's
	// tier via space. Safe is always included; caution is opt-in.
	includeCaution bool

	// drill-in: when >=0, show the selected finding's detail pane.
	detail int

	// clean lifecycle.
	cleaning   bool
	cleanProof string
	cleanErr   error
	confirm    bool // c pressed once: confirm modal armed (Rule A1)
}

func newWasteScreen() *wasteScreen {
	home, _ := os.UserHomeDir()
	return &wasteScreen{state: stateIdle, home: home, detail: -1}
}

func (s *wasteScreen) Name() string     { return "Waste" }
func (s *wasteScreen) Sigil() string    { return "focus-marker" } // Jackal hunter (safe sigil)
func (s *wasteScreen) Layout() Layout   { return LayoutInspect }
func (s *wasteScreen) State() loadState { return s.state }

func (s *wasteScreen) HintIDs() []CommandID {
	return []CommandID{CmdMoveDown, CmdInspect, CmdToggle, CmdClean, CmdScan}
}

func (s *wasteScreen) RightMeta() string {
	if s.state != stateReady {
		return ""
	}
	return fmt.Sprintf("%s reclaimable", fmtBytes(s.report.ReclaimableSize))
}

func (s *wasteScreen) Load() tea.Cmd {
	s.state = stateLoading
	return func() tea.Msg {
		var r scanReport
		err := decode("scan", &r)
		return wasteLoaded{report: r, err: err}
	}
}

type wasteLoaded struct {
	report scanReport
	err    error
}

func (s *wasteScreen) Update(msg tea.Msg, caps Capabilities) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case wasteLoaded:
		if m.err != nil {
			s.state = stateError
			s.err = m.err
			return s, nil
		}
		s.state = stateReady
		s.report = m.report
		sortFindings(s.report.Findings)
		s.selected = clampSelection(s.selected, len(s.report.Findings))
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
		// Rescan so the review list reflects what was freed.
		return s, s.Load()

	case keyMsg:
		return s.handleCmd(m.cmd, caps)
	}
	return s, nil
}

func (s *wasteScreen) handleCmd(cmd Command, caps Capabilities) (Screen, tea.Cmd) {
	n := len(s.report.Findings)
	// A pending confirm captures the next decision (Rule A1: destructive never
	// fires from one key).
	if s.confirm {
		switch cmd.ID {
		case CmdInspect: // enter = deliberate second confirmation → clean
			s.confirm = false
			s.cleaning = true
			s.cleanErr = nil
			include := s.includeCaution
			return s, runCmd("clean", func() (any, error) {
				var r cleanReport
				args := []string{"--confirm", "--yes"}
				if include {
					args = append(args, "--include-caution")
				}
				err := decode("clean", &r, args...)
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
			s.detail = -1 // toggle detail off
		} else {
			s.detail = s.selected
		}
	case CmdBack:
		s.detail = -1
	case CmdToggle:
		// Toggle the caution-tier inclusion when the selected item is caution-tier;
		// safe items are always included, protected never. Honest: the toggle maps
		// to the real --include-caution scope switch.
		if n > 0 {
			f := s.report.Findings[s.selected]
			if strings.EqualFold(f.Severity, "caution") {
				s.includeCaution = !s.includeCaution
			}
		}
	case CmdScan:
		return s, s.Load()
	case CmdClean:
		if n == 0 {
			return s, nil
		}
		s.confirm = true // arm confirm modal; do not execute
	}
	return s, nil
}

func (s *wasteScreen) View(width, height int, caps Capabilities) []string {
	switch s.state {
	case stateIdle, stateLoading:
		return loadingLines("scanning for waste…", caps)
	case stateError:
		return errorLines(s.err, caps)
	}
	if len(s.report.Findings) == 0 {
		return emptyLines("no reclaimable waste found — your machine is clean", caps)
	}

	// Confirm modal takes over the body (Rule A1 second confirmation).
	if s.confirm {
		return s.confirmLines(caps)
	}

	lines := make([]string, 0, height)
	scope := "safe tier"
	if s.includeCaution {
		scope = "safe + caution tiers"
	}
	lines = append(lines,
		"  "+Paint(fmt.Sprintf("%d findings · %s reclaimable · cleaning %s",
			len(s.report.Findings), fmtBytes(s.report.ReclaimableSize), scope), TokBrand, caps),
		"")

	cols := []Column{
		{Title: "RULE", Width: 26, Align: AlignLeft},
		{Title: "PATH", Width: 34, Align: AlignLeft},
		{Title: "SIZE", Width: 9, Align: AlignRight},
		{Title: "TIER", Width: 9, Align: AlignLeft},
	}
	rows := make([]listRow, 0, len(s.report.Findings))
	for i, f := range s.report.Findings {
		tierLabel := scanSeverityToken(f.Severity).SeverityLabel()
		if strings.EqualFold(f.Severity, "protected") {
			tierLabel = "BLOCK"
		}
		rows = append(rows, listRow{
			cells: []string{
				f.Description,
				shortPath(f.Path, s.home),
				fmtBytes(f.SizeBytes),
				tierLabel,
			},
			token:    scanSeverityToken(f.Severity),
			selected: i == s.selected,
			checked:  s.included(f),
		})
	}
	// Cap rows to available height, keeping the selection visible.
	bodyLines := renderTable(cols, rows, caps, true)
	lines = append(lines, bodyLines...)

	// Detail drill-in.
	if s.detail >= 0 && s.detail < len(s.report.Findings) {
		lines = append(lines, s.detailLines(caps)...)
	}

	// Clean result / offer.
	lines = append(lines, "")
	switch {
	case s.cleaning:
		lines = append(lines, "  "+Paint(Sigil("spinner-static", caps)+" cleaning…", TokDim, caps))
	case s.cleanErr != nil:
		lines = append(lines, "  "+Paint("BLOCK clean failed: "+s.cleanErr.Error(), TokDanger, caps))
	case s.cleanProof != "":
		lines = append(lines, "  "+Paint(Sigil("check", caps)+" "+s.cleanProof, TokOK, caps))
	default:
		lines = append(lines, "  "+Paint("space toggles caution tier · enter drills in · c cleans", TokDim, caps))
	}
	return lines
}

// included reports whether a finding is in the current clean scope: safe always,
// caution when the toggle is on, protected never.
func (s *wasteScreen) included(f scanFinding) bool {
	switch strings.ToLower(f.Severity) {
	case "protected":
		return false
	case "caution":
		return s.includeCaution
	default:
		return true
	}
}

func (s *wasteScreen) detailLines(caps Capabilities) []string {
	f := s.report.Findings[s.detail]
	return []string{
		"",
		"  " + Paint("── "+f.Description+" ", TokAccent, caps),
		"  " + Paint("path:   ", TokDim, caps) + f.Path,
		"  " + Paint("size:   ", TokDim, caps) + fmt.Sprintf("%s · %d files", fmtBytes(f.SizeBytes), f.FileCount),
		"  " + Paint("tier:   ", TokDim, caps) + Paint(strings.ToUpper(f.Severity), scanSeverityToken(f.Severity), caps),
	}
}

func (s *wasteScreen) confirmLines(caps Capabilities) []string {
	scope := "safe tier"
	if s.includeCaution {
		scope = "safe + caution tiers"
	}
	return []string{
		"",
		"  " + Paint("CONFIRM CLEAN", TokWarn, caps),
		"",
		fmt.Sprintf("  This will move the %s (%s) to the Trash.",
			scope, fmtBytes(s.report.ReclaimableSize)),
		"  Items go to Trash first — recoverable until you empty it.",
		"",
		"  " + Paint("enter", TokBrand, caps) + Paint("  confirm and clean", TokDim, caps),
		"  " + Paint("esc", TokBrand, caps) + Paint("    cancel — nothing is removed", TokDim, caps),
	}
}

// sortFindings orders findings by size descending so the biggest reclaimable
// wins the top rows (operator attention economy).
func sortFindings(fs []scanFinding) {
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].SizeBytes > fs[j].SizeBytes })
}

// cleanProofFrom extracts the freed-space proof from the clean report's summary
// or evidence.
func cleanProofFrom(r cleanReport) string {
	if r.Summary != "" {
		return r.Summary
	}
	for _, e := range r.Evidence {
		if strings.Contains(strings.ToLower(e.Label), "reclaim") || strings.Contains(strings.ToLower(e.Label), "freed") {
			return e.Label + ": " + e.Value
		}
	}
	return "cleanup complete"
}
