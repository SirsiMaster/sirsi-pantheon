package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Pulse — the memory-first home screen (project_pantheon_memory_first_view).
//
// RAM is the pre-eminent view: a determinate gauge, the live pressure level, and
// the named top memory hogs, read from `sirsi vitals --json` (built for a 2s
// tick). The one action is relief: `r` runs `sirsi relieve --memory --confirm`
// (proof §3.2: "r relieve") and the screen shows the before/after free-memory
// delta inline plus a toast. Relief is FixRelief-honest — it eases a live cause;
// it never claims to have killed anything.

const pulseTick = 2 * time.Second

// pulseTickMsg drives the 2s auto-refresh of the gauge.
type pulseTickMsg struct{}

// pulseLoaded carries a vitals snapshot back to the screen.
type pulseLoaded struct {
	report vitalsReport
	err    error
}

type pulseScreen struct {
	state  loadState
	err    error
	vitals vitalsReport

	// relief result, shown inline after `f`.
	relieving   bool
	reliefDelta string // e.g. "15.2 GB -> 17.9 GB free"
	reliefErr   error
}

func newPulseScreen() *pulseScreen { return &pulseScreen{state: stateIdle} }

func (s *pulseScreen) Name() string     { return "Pulse" }
func (s *pulseScreen) Sigil() string    { return "horus" } // memory lord surface
func (s *pulseScreen) Layout() Layout   { return LayoutSurvey }
func (s *pulseScreen) State() loadState { return s.state }

func (s *pulseScreen) HintIDs() []CommandID {
	return []CommandID{CmdRelieve, CmdRefresh, CmdTab, CmdHelp, CmdQuit}
}

func (s *pulseScreen) RightMeta() string {
	if s.state != stateReady || s.vitals.TotalBytes == 0 {
		return ""
	}
	return fmt.Sprintf("%d%% used", usedPct(s.vitals))
}

// Load fetches a vitals snapshot. It does NOT schedule the recurring 2s tick —
// the App runloop owns that (pulseScheduleTick, delivered as a pulseTickMsg) so
// Load stays a pure, non-blocking data fetch that tests can drive directly.
func (s *pulseScreen) Load() tea.Cmd {
	s.state = stateLoading
	return loadVitals()
}

func loadVitals() tea.Cmd {
	return func() tea.Msg {
		var r vitalsReport
		err := decode("vitals", &r, "--top", "5")
		return pulseLoaded{report: r, err: err}
	}
}

// pulseScheduleTick arms the recurring 2s gauge refresh. It is owned by the App
// runloop, not by Load, so no blocking timer is ever invoked synchronously.
func pulseScheduleTick() tea.Cmd {
	return tea.Tick(pulseTick, func(time.Time) tea.Msg { return pulseTickMsg{} })
}

func (s *pulseScreen) Update(msg tea.Msg, caps Capabilities) (Screen, tea.Cmd) {
	switch m := msg.(type) {
	case pulseLoaded:
		if m.err != nil {
			s.state = stateError
			s.err = m.err
			return s, nil
		}
		s.state = stateReady
		s.vitals = m.report
		return s, nil

	case pulseTickMsg:
		// Auto-refresh only once we have first data (don't stack requests before
		// the first frame). The App re-arms the tick; the screen only reloads.
		if s.state == stateReady {
			return s, loadVitals()
		}
		return s, nil

	case dispatchDone:
		if m.kind != "relieve" {
			return s, nil
		}
		s.relieving = false
		if m.err != nil {
			s.reliefErr = m.err
			return s, nil
		}
		if rep, ok := m.report.(cleanReport); ok {
			s.reliefDelta = reliefDeltaFrom(rep)
		}
		// Refresh the gauge to reflect the relief.
		return s, loadVitals()

	case keyMsg:
		switch m.cmd.ID {
		case CmdRefresh:
			return s, loadVitals()
		case CmdRelieve:
			if s.relieving {
				return s, nil
			}
			s.relieving = true
			s.reliefErr = nil
			return s, runCmd("relieve", func() (any, error) {
				var r cleanReport
				err := decode("relieve", &r, "--memory", "--confirm")
				return r, err
			})
		}
	}
	return s, nil
}

func (s *pulseScreen) View(width, height int, caps Capabilities) []string {
	switch s.state {
	case stateIdle, stateLoading:
		return loadingLines("sampling memory…", caps)
	case stateError:
		return errorLines(s.err, caps)
	}

	v := s.vitals
	pct := usedPct(v)
	barW := width - 24
	if barW > 48 {
		barW = 48
	}
	if barW < 12 {
		barW = 12
	}
	gaugeTok := pressureToken(v.Pressure)

	lines := []string{
		"",
		"  " + Paint("MEMORY", TokBrand, caps),
		fmt.Sprintf("  %s   %s of %s used · %s free",
			gauge(pct, barW, gaugeTok, caps),
			fmtBytes(v.UsedBytes), fmtBytes(v.TotalBytes), fmtBytes(v.FreeBytes)),
		fmt.Sprintf("  pressure: %s   swap in use: %s",
			Paint(strings.ToUpper(v.Pressure), gaugeTok, caps), fmtBytes(v.SwapUsedBytes)),
		"",
		"  " + Paint("TOP MEMORY USERS", TokBrand, caps),
	}

	cols := []Column{
		{Title: "PROCESS", Width: 28, Align: AlignLeft},
		{Title: "PID", Width: 8, Align: AlignRight},
		{Title: "MEMORY", Width: 10, Align: AlignRight},
	}
	rows := make([]listRow, 0, len(v.Top))
	for _, p := range v.Top {
		rows = append(rows, listRow{
			cells: []string{p.Name, fmt.Sprintf("%d", p.PID), fmtBytes(p.RSSBytes)},
			token: TokDim,
		})
	}
	if len(rows) == 0 {
		lines = append(lines, "  "+Paint("no notable memory users", TokDim, caps))
	} else {
		lines = append(lines, renderTable(cols, rows, caps, false)...)
	}

	// Relief status block — the before/after proof, or an error, or the offer.
	lines = append(lines, "")
	switch {
	case s.relieving:
		lines = append(lines, "  "+Paint(Sigil("spinner-static", caps)+" relieving memory pressure…", TokDim, caps))
	case s.reliefErr != nil:
		lines = append(lines, "  "+Paint("BLOCK relief failed: "+s.reliefErr.Error(), TokDanger, caps))
	case s.reliefDelta != "":
		lines = append(lines, "  "+Paint(Sigil("check", caps)+" relieved · "+s.reliefDelta, TokOK, caps))
	default:
		lines = append(lines, "  "+Paint("press r to flush inactive caches and ease pressure", TokDim, caps))
	}
	return lines
}

// usedPct is the used-RAM percentage (0..100).
func usedPct(v vitalsReport) int {
	if v.TotalBytes <= 0 {
		return 0
	}
	return int(v.UsedBytes * 100 / v.TotalBytes)
}

// pressureToken colors the gauge/pressure by level. normal=ok, warn=warn,
// critical=danger. Unknown reads neutral, never alarming.
func pressureToken(p string) Token {
	switch strings.ToLower(p) {
	case "normal":
		return TokOK
	case "warn", "warning":
		return TokWarn
	case "critical":
		return TokDanger
	default:
		return TokDim
	}
}

// reliefDeltaFrom pulls the before/after free-memory evidence out of the relieve
// report into a compact "X -> Y free" delta.
func reliefDeltaFrom(r cleanReport) string {
	var before, after string
	for _, e := range r.Evidence {
		switch strings.ToLower(e.Label) {
		case "free before":
			before = e.Value
		case "free after":
			after = e.Value
		}
	}
	if before != "" && after != "" {
		return fmt.Sprintf("%s %s %s free", before, arrow(), after)
	}
	if r.Summary != "" {
		return r.Summary
	}
	return "memory pressure eased"
}

// arrow returns a plain-ASCII arrow for the delta (grid-safe everywhere).
func arrow() string { return "->" }
