package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Health — `sirsi diagnose --json` findings, each with its HONEST one-key fix
// (ADR-033: never present a monitor as a fix).
//
// Each finding carries a severity, a message, and — when remediable — a Fix
// command classified by FixKind:
//
//	instant  → the fix provably changes this finding's state now (label "Fix").
//	relief   → the fix eases a live cause; a (7d) historical count won't drop
//	           retroactively (label "Relieve", honest).
//	guidance → the fix only acts if the condition is live; otherwise it prints
//	           guidance and is a no-op — NOT offered as a one-key fix (ADR-033).
//
// `f` applies the selected finding's fix by dispatching its exact Fix command.

type healthScreen struct {
	state    loadState
	err      error
	report   diagReport
	selected int
	detail   int

	fixing bool
	fixMsg string
	fixErr error
}

func newHealthScreen() *healthScreen { return &healthScreen{state: stateIdle, detail: -1} }

func (s *healthScreen) Name() string     { return "Health" }
func (s *healthScreen) Sigil() string    { return "check" } // Ma'at order (safe sigil)
func (s *healthScreen) Layout() Layout   { return LayoutInspect }
func (s *healthScreen) State() loadState { return s.state }

func (s *healthScreen) HintIDs() []CommandID {
	return []CommandID{CmdMoveDown, CmdInspect, CmdFix, CmdDiag, CmdTab}
}

func (s *healthScreen) RightMeta() string {
	if s.state != stateReady {
		return ""
	}
	attn := 0
	for _, f := range s.report.Findings {
		if f.Severity >= 1 {
			attn++
		}
	}
	if attn == 0 {
		return "all healthy"
	}
	return fmt.Sprintf("%d need attention", attn)
}

func (s *healthScreen) Load() tea.Cmd {
	s.state = stateLoading
	return func() tea.Msg {
		var r diagReport
		err := decode("diagnose", &r)
		return healthLoaded{report: r, err: err}
	}
}

type healthLoaded struct {
	report diagReport
	err    error
}

func (s *healthScreen) Update(msg tea.Msg, caps Capabilities) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case healthLoaded:
		if m.err != nil {
			s.state = stateError
			s.err = m.err
			return s, nil
		}
		s.state = stateReady
		s.report = m.report
		sortFindingsBySeverity(s.report.Findings)
		s.selected = clampSelection(s.selected, len(s.report.Findings))
		return s, nil

	case dispatchDone:
		if m.kind != "fix" {
			return s, nil
		}
		s.fixing = false
		if m.err != nil {
			s.fixErr = m.err
			return s, nil
		}
		if rep, ok := m.report.(cleanReport); ok && rep.Summary != "" {
			s.fixMsg = rep.Summary
		} else {
			s.fixMsg = "fix applied"
		}
		return s, s.Load() // re-diagnose to show the new state honestly

	case keyMsg:
		return s.handleCmd(m.cmd)
	}
	return s, nil
}

func (s *healthScreen) handleCmd(cmd Command) (Screen, tea.Cmd) {
	n := len(s.report.Findings)
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
	case CmdDiag, CmdRefresh:
		return s, s.Load()
	case CmdFix:
		if n == 0 {
			return s, nil
		}
		f := s.report.Findings[s.selected]
		if !hasOfferableFix(f) {
			s.fixErr = fmt.Errorf("no one-key fix for %q — see detail for guidance", f.Check)
			return s, nil
		}
		s.fixing = true
		s.fixErr = nil
		fixCmd := f.Fix
		return s, runCmd("fix", func() (any, error) {
			var r cleanReport
			// The Fix is a full sirsi command string, e.g. "sirsi self-update" or
			// "sirsi clean --include-caution". Run it verb+args through the seam.
			verb, args := splitFixCommand(fixCmd)
			err := decode(verb, &r, args...)
			return r, err
		})
	}
	return s, nil
}

func (s *healthScreen) View(width, height int, caps Capabilities) []string {
	switch s.state {
	case stateIdle, stateLoading:
		return loadingLines("running diagnostics…", caps)
	case stateError:
		return errorLines(s.err, caps)
	}
	if len(s.report.Findings) == 0 {
		return emptyLines("no diagnostics returned", caps)
	}

	lines := []string{
		"  " + Paint(fmt.Sprintf("%d checks · %s", len(s.report.Findings), s.report.Duration), TokBrand, caps),
		"",
	}
	cols := []Column{
		{Title: "CHECK", Width: 24, Align: AlignLeft},
		{Title: "MESSAGE", Width: 40, Align: AlignLeft},
		{Title: "STATUS", Width: 7, Align: AlignLeft},
	}
	rows := make([]listRow, 0, len(s.report.Findings))
	for i, f := range s.report.Findings {
		tok := severityToken(f.Severity)
		rows = append(rows, listRow{
			cells:    []string{f.Check, f.Message, tok.SeverityLabel()},
			token:    tok,
			selected: i == s.selected,
		})
	}
	lines = append(lines, renderTable(cols, rows, caps, false)...)

	if s.detail >= 0 && s.detail < len(s.report.Findings) {
		lines = append(lines, s.detailLines(caps)...)
	}

	lines = append(lines, "")
	switch {
	case s.fixing:
		lines = append(lines, "  "+Paint(Sigil("spinner-static", caps)+" applying fix…", TokDim, caps))
	case s.fixErr != nil:
		lines = append(lines, "  "+Paint("WARN "+s.fixErr.Error(), TokWarn, caps))
	case s.fixMsg != "":
		lines = append(lines, "  "+Paint(Sigil("check", caps)+" "+s.fixMsg, TokOK, caps))
	default:
		lines = append(lines, "  "+Paint(s.fixHintForSelection(), TokDim, caps))
	}
	return lines
}

// fixHintForSelection tells the operator, honestly, what f will do to the
// selected finding — matching the FixKind so the label never over-promises.
func (s *healthScreen) fixHintForSelection() string {
	if len(s.report.Findings) == 0 {
		return "enter inspects · d re-runs diagnostics"
	}
	f := s.report.Findings[s.selected]
	switch {
	case f.Fix == "":
		return "no fix needed — enter for detail"
	case f.FixKind == "instant":
		return "f fixes this now · " + f.Fix
	case f.FixKind == "relief":
		return "f relieves the live cause (history won't drop) · " + f.Fix
	case f.FixKind == "guidance":
		return "no one-key fix — f only acts if the condition is live now"
	default:
		return "f runs · " + f.Fix
	}
}

func (s *healthScreen) detailLines(caps Capabilities) []string {
	f := s.report.Findings[s.detail]
	out := []string{
		"",
		"  " + Paint("── "+f.Check+" ", TokAccent, caps),
		"  " + Paint("status: ", TokDim, caps) + Paint(severityToken(f.Severity).SeverityLabel(), severityToken(f.Severity), caps),
		"  " + Paint("what:   ", TokDim, caps) + f.Message,
	}
	if f.Detail != "" {
		out = append(out, "  "+Paint("detail: ", TokDim, caps)+f.Detail)
	}
	if f.Trend && f.ActiveDays > 0 {
		out = append(out, "  "+Paint("trend:  ", TokDim, caps)+fmt.Sprintf("recurred on %d of the last 7 days", f.ActiveDays))
	}
	if f.Fix != "" {
		out = append(out, "  "+Paint("fix:    ", TokDim, caps)+f.Fix+"  "+Paint("("+fixKindLabel(f.FixKind)+")", TokDim, caps))
	} else {
		out = append(out, "  "+Paint("fix:    ", TokDim, caps)+Paint("none needed — informational", TokDim, caps))
	}
	return out
}

// hasOfferableFix reports whether a finding's fix should be offered as a one-key
// action. Guidance-kind fixes are NOT offered (ADR-033: a guidance no-op must
// not masquerade as a fix); instant and relief are.
func hasOfferableFix(f diagFinding) bool {
	return f.Fix != "" && f.FixKind != "guidance"
}

// fixKindLabel is the honest human label for a FixKind.
func fixKindLabel(kind string) string {
	switch kind {
	case "instant":
		return "fixes now"
	case "relief":
		return "relieves live cause"
	case "guidance":
		return "guidance only"
	default:
		return "runs a command"
	}
}

// splitFixCommand parses a "sirsi <verb> [args…]" fix string into the verb and
// its args (dropping the leading "sirsi"), for dispatch through the runner seam.
func splitFixCommand(cmd string) (string, []string) {
	fields := strings.Fields(cmd)
	// Drop a leading "sirsi" if present.
	if len(fields) > 0 && fields[0] == "sirsi" {
		fields = fields[1:]
	}
	if len(fields) == 0 {
		return "diagnose", nil
	}
	return fields[0], fields[1:]
}

// sortFindingsBySeverity orders findings critical → attention → ok so the items
// that need the operator are at the top.
func sortFindingsBySeverity(fs []diagFinding) {
	// stable insertion by descending severity
	for i := 1; i < len(fs); i++ {
		for j := i; j > 0 && fs[j].Severity > fs[j-1].Severity; j-- {
			fs[j], fs[j-1] = fs[j-1], fs[j]
		}
	}
}
