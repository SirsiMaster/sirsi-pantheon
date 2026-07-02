package tui

import (
	"fmt"
	"os"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Waste — scan → review → clean, with per-item selection and a freed-space proof
// (menubar #130 flow).
//
// `sirsi scan --json` produces the per-item review list. Each item shows its
// rule, size, and tier; the operator toggles INDIVIDUAL rows (space), drills into
// a finding (enter) to see its full path, then cleans (c) exactly the checked set.
//
// Honest dispatch (Rule A1 — preview == apply): cleaning routes through
// `sirsi anubis clean --only <each checked path> --confirm --yes`. The Go layer's
// narrowToPaths intersects `--only` with the safe/caution clean target, so the
// APPLIED set is, by construction, exactly the CHECKED set — nothing more. The
// confirm modal's stated GB is the sum of the checked rows, so the number shown
// equals what moves to Trash. Rows the Go layer refuses (warning tier, or AI
// model weights excluded from the waste headline) are BLOCK: they can never be
// checked, so they can never be dispatched even by an errant --only. If any
// checked row is caution-tier, --include-caution is added so narrowToPaths keeps
// it (otherwise caution rows would be dropped from the safe-only target).

type wasteScreen struct {
	state    loadState
	err      error
	report   scanReport
	home     string
	selected int

	// checked is the per-item selection, keyed by finding Path. Only cleanable
	// rows (safe/caution, non-AI) can be checked (see cleanable). The APPLIED set
	// is exactly this set (dispatched as repeated --only paths), so the confirm
	// total == the checked total == what is cleaned (Rule A1).
	checked map[string]bool

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
	return &wasteScreen{state: stateIdle, home: home, detail: -1, checked: map[string]bool{}}
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
		// Drop checks for paths no longer in the scan (e.g. after a clean), so the
		// selection never references stale findings.
		s.pruneChecks()
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
		// Cleaned paths are gone; clear the selection and rescan so the review list
		// reflects what was freed.
		s.checked = map[string]bool{}
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
			args := s.cleanArgs()
			return s, runCmd("clean", func() (any, error) {
				var r cleanReport
				err := decode("anubis", &r, args...)
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
		// Toggle the SELECTED row's per-item checkbox. Only cleanable rows can be
		// checked — a BLOCK row (warning tier or AI model weights) never enters the
		// selection, so the checked set is always a valid clean subset (Rule A1).
		if n > 0 {
			f := s.report.Findings[s.selected]
			if cleanable(f) {
				s.checked[f.Path] = !s.checked[f.Path]
			}
		}
	case CmdRefresh, CmdScan:
		return s, s.Load()
	case CmdClean:
		if s.checkedCount() == 0 {
			return s, nil // nothing selected → nothing to confirm
		}
		s.confirm = true // arm confirm modal; do not execute
	}
	return s, nil
}

// cleanArgs builds the exact `anubis clean` argument vector for the checked set:
// `anubis clean --only <p1> --only <p2> … [--include-caution] --confirm --yes`.
// One --only per checked path makes the applied set == the checked set (Rule A1);
// --include-caution is added iff a caution-tier row is checked, so narrowToPaths
// keeps it in scope. The runner appends --json.
func (s *wasteScreen) cleanArgs() []string {
	args := []string{"clean"}
	anyCaution := false
	// Deterministic order: findings are already sorted by size; emit --only in
	// that order so the dispatched command is stable (tests can pin it).
	for _, f := range s.report.Findings {
		if !s.checked[f.Path] {
			continue
		}
		args = append(args, "--only", f.Path)
		if strings.EqualFold(f.Severity, "caution") {
			anyCaution = true
		}
	}
	if anyCaution {
		args = append(args, "--include-caution")
	}
	args = append(args, "--confirm", "--yes")
	return args
}

// checkedCount is the number of currently-checked rows.
func (s *wasteScreen) checkedCount() int {
	n := 0
	for _, f := range s.report.Findings {
		if s.checked[f.Path] {
			n++
		}
	}
	return n
}

// checkedBytes is the sum of the checked rows' sizes — the exact figure the
// confirm modal shows, and (by --only dispatch) exactly what moves to Trash.
func (s *wasteScreen) checkedBytes() int64 {
	var total int64
	for _, f := range s.report.Findings {
		if s.checked[f.Path] {
			total += f.SizeBytes
		}
	}
	return total
}

// pruneChecks drops any checked path that is no longer present (or no longer
// cleanable) in the current scan, keeping the selection a valid clean subset.
func (s *wasteScreen) pruneChecks() {
	if len(s.checked) == 0 {
		return
	}
	live := make(map[string]bool, len(s.report.Findings))
	for _, f := range s.report.Findings {
		if cleanable(f) {
			live[f.Path] = true
		}
	}
	for p := range s.checked {
		if !live[p] {
			delete(s.checked, p)
		}
	}
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
	lines = append(lines,
		"  "+Paint(fmt.Sprintf("%d findings · %s reclaimable · %d selected (%s)",
			len(s.report.Findings), fmtBytes(s.report.ReclaimableSize),
			s.checkedCount(), fmtBytes(s.checkedBytes())), TokBrand, caps),
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
		if !cleanable(f) {
			tierLabel = "BLOCK" // warning tier or AI model weights — never cleanable
		}
		rows = append(rows, listRow{
			cells: []string{
				f.Description,
				shortPath(f.Path, s.home),
				fmtBytes(f.SizeBytes),
				tierLabel,
			},
			token:     wasteRowToken(f),
			selected:  i == s.selected,
			checked:   s.checked[f.Path],
			checkable: cleanable(f),
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
		lines = append(lines, "  "+Paint("space checks the selected item · enter drills in · c cleans the checked set", TokDim, caps))
	}
	return lines
}

// cleanable reports whether a finding may be cleaned — and therefore checked. A
// finding is cleanable only if the Go layer would actually remove it: safe or
// caution tier AND not an AI model-weights category. Warning-tier (data/config
// that may break) is never auto-cleaned (selectCleanTargets), and CategoryAI is
// excluded from the reclaimable-waste headline and the one-click cleaner (67 GB
// cold Gemma weights are not trash). A non-cleanable row is BLOCK: it cannot be
// checked, so it can never enter a --only dispatch — preview == apply holds even
// if the operator tries to select it.
func cleanable(f scanFinding) bool {
	if strings.EqualFold(f.Category, "ai") {
		return false
	}
	switch strings.ToLower(f.Severity) {
	case "safe", "caution":
		return true
	default: // warning or anything unexpected
		return false
	}
}

// wasteRowToken colors a row: BLOCK rows read danger, cleanable rows by tier.
func wasteRowToken(f scanFinding) Token {
	if !cleanable(f) {
		return TokDanger
	}
	return scanSeverityToken(f.Severity)
}

func (s *wasteScreen) detailLines(caps Capabilities) []string {
	f := s.report.Findings[s.detail]
	out := []string{
		"",
		"  " + Paint("── "+f.Description+" ", TokAccent, caps),
		"  " + Paint("path:   ", TokDim, caps) + f.Path,
		"  " + Paint("size:   ", TokDim, caps) + fmt.Sprintf("%s · %d files", fmtBytes(f.SizeBytes), f.FileCount),
		"  " + Paint("tier:   ", TokDim, caps) + Paint(strings.ToUpper(f.Severity), scanSeverityToken(f.Severity), caps),
	}
	if !cleanable(f) {
		reason := "warning tier — may break data/config; never auto-cleaned"
		if strings.EqualFold(f.Category, "ai") {
			reason = "AI model weights — excluded from one-click clean (expensive to regenerate)"
		}
		out = append(out, "  "+Paint("status: ", TokDim, caps)+Paint("BLOCK — "+reason, TokDanger, caps))
	}
	return out
}

func (s *wasteScreen) confirmLines(caps Capabilities) []string {
	n := s.checkedCount()
	return []string{
		"",
		"  " + Paint("CONFIRM CLEAN", TokWarn, caps),
		"",
		fmt.Sprintf("  This will move %d selected item(s) (%s) to the Trash.",
			n, fmtBytes(s.checkedBytes())),
		"  Exactly the checked items are cleaned — nothing else.",
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
