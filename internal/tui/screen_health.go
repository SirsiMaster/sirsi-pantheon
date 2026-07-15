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

	// confirm is armed when the selected finding's fix is DESTRUCTIVE (moves data
	// to Trash / thins snapshots): f arms the modal, a second enter applies it
	// (Rule A1). Non-destructive fixes (self-update, relieve, spotlight-exclude)
	// bypass the modal — but every applied fix carries the flags that make it
	// actually APPLY, never a preview no-op (ADR-033).
	confirm bool
}

func newHealthScreen() *healthScreen { return &healthScreen{state: stateIdle, detail: -1} }

func (s *healthScreen) Name() string     { return "Health" }
func (s *healthScreen) Sigil() string    { return "check" } // Ma'at order (safe sigil)
func (s *healthScreen) Layout() Layout   { return LayoutInspect }
func (s *healthScreen) State() loadState { return s.state }

// Busy reports an in-flight operation: diagnostics loading or a dispatched fix
// not yet resolved (quit guard, P2#8).
func (s *healthScreen) Busy() bool { return s.state == stateLoading || s.fixing }

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
	// A pending confirm captures the next decision for a DESTRUCTIVE fix (Rule A1:
	// a Trash-deleting fix never fires from one key).
	if s.confirm {
		switch cmd.ID {
		case CmdFix, CmdInspect: // f again, or enter = deliberate second confirmation
			s.confirm = false
			return s, s.dispatchFix(s.report.Findings[s.selected])
		case CmdBack:
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
		// Destructive fixes (clean / reclaim-snapshots) route through the confirm
		// modal; f only arms it. Non-destructive fixes apply immediately.
		if plan := fixPlan(f.Fix); plan.destructive {
			s.confirm = true
			s.fixErr = nil
			return s, nil
		}
		return s, s.dispatchFix(f)
	}
	return s, nil
}

// dispatchFix runs the finding's fix with the flags that make it actually APPLY
// (never a preview no-op — the ADR-033 trap). The plan encodes, per verb, exactly
// which apply flags the CLI accepts: clean gets --confirm --yes, reclaim-snapshots
// and relieve get --confirm (they reject --yes), self-update and spotlight-exclude
// run verbatim. Dispatch flows through the injectable runner seam.
func (s *healthScreen) dispatchFix(f diagFinding) tea.Cmd {
	s.fixing = true
	s.fixErr = nil
	plan := fixPlan(f.Fix)
	return runCmd("fix", func() (any, error) {
		var r cleanReport
		err := decode(plan.verb, &r, plan.args...)
		return r, err
	})
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

	// Confirm modal takes over the body for a destructive fix (Rule A1).
	if s.confirm {
		return s.confirmLines(caps)
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
	// Tail first, so the table window's line budget is exact (P2#6).
	var tail []string
	if s.detail >= 0 && s.detail < len(s.report.Findings) {
		tail = append(tail, s.detailLines(caps)...)
	}

	tail = append(tail, "")
	switch {
	case s.fixing:
		tail = append(tail, "  "+Paint(Sigil("spinner-static", caps)+" applying fix…", TokDim, caps))
	case s.fixErr != nil:
		tail = append(tail, "  "+Paint("WARN "+s.fixErr.Error(), TokWarn, caps))
	case s.fixMsg != "":
		tail = append(tail, "  "+Paint(Sigil("check", caps)+" "+s.fixMsg, TokOK, caps))
	default:
		tail = append(tail, "  "+Paint(s.fixHintForSelection(), TokDim, caps))
	}

	lines = append(lines, renderTableWindow(cols, rows, caps, false, height-len(lines)-len(tail))...)
	lines = append(lines, tail...)
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
	case fixPlan(f.Fix).destructive:
		return "f cleans this (confirm first) · " + f.Fix
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

// confirmLines renders the destructive-fix confirmation (Rule A1): it names the
// exact command that will run so the operator sees precisely what applies.
func (s *healthScreen) confirmLines(caps Capabilities) []string {
	f := s.report.Findings[s.selected]
	return []string{
		"",
		"  " + Paint("CONFIRM FIX", TokWarn, caps),
		"",
		fmt.Sprintf("  This applies the fix for %q:", f.Check),
		"  " + Paint(f.Fix, TokBrand, caps),
		"  Destructive items move to Trash first — recoverable until you empty it.",
		"",
		"  " + Paint("enter", TokBrand, caps) + Paint("  confirm and apply", TokDim, caps),
		"  " + Paint("esc", TokBrand, caps) + Paint("    cancel — nothing changes", TokDim, caps),
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

// healthFixPlan is the resolved dispatch for a finding's fix: the verb, the exact
// args (the fix's own args PLUS the apply flags the verb accepts), and whether it
// is destructive (must route through the confirm modal).
type healthFixPlan struct {
	verb        string
	args        []string
	destructive bool
}

// fixPlan turns a diagnose Fix string into a dispatch that actually APPLIES the
// fix (never a preview no-op — the ADR-033 "Fix applied but nothing changed"
// trap) using only flags the target verb accepts:
//
//	self-update       → verbatim (instant; replaces the drifted binary itself).
//	clean [args…]     → + --confirm --yes (destructive: moves to Trash).
//	reclaim-snapshots → + --confirm       (destructive: thins snapshots; no --yes flag).
//	relieve [args…]   → + --confirm       (relief: eases a live cause; no --yes flag).
//	spotlight-exclude → verbatim          (config change; the fix string has no --json).
//	anything else     → verbatim.
//
// Only clean and reclaim-snapshots (Trash / disk deletion) are flagged
// destructive, so only they gate on the confirm modal (Rule A1). --confirm/--yes
// are appended by verb allow-list, never blindly, because relieve and
// reclaim-snapshots REJECT --yes and would error on an unknown flag.
func fixPlan(fix string) healthFixPlan {
	verb, args := splitFixCommand(fix)
	switch verb {
	case "clean":
		return healthFixPlan{verb: verb, args: append(args, "--confirm", "--yes"), destructive: true}
	case "reclaim-snapshots":
		return healthFixPlan{verb: verb, args: append(args, "--confirm"), destructive: true}
	case "relieve":
		return healthFixPlan{verb: verb, args: append(args, "--confirm"), destructive: false}
	default:
		// self-update, spotlight-exclude, and any other verb apply verbatim.
		return healthFixPlan{verb: verb, args: args, destructive: false}
	}
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
