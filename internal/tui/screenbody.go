package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Shared screen-body helpers. Every screen renders an explicit loading / empty /
// error state (quality bar: no blank frames, every state accounted for) and a
// selectable list. Centralizing these keeps the five screens thin and their
// states consistent.

// loadingLines renders the in-flight state: a static spinner sigil and a label.
// Motion is off by construction here (the sigil is static), honoring reduced-
// motion without a timer.
func loadingLines(label string, caps Capabilities) []string {
	return []string{
		"",
		"  " + Paint(Sigil("spinner-static", caps)+" "+label, TokDim, caps),
	}
}

// errorLines renders an in-pane error banner (§4): the error's message with a
// danger token and a retry hint. The reason is shown, never swallowed.
func errorLines(err error, caps Capabilities) []string {
	msg := "unknown error"
	if err != nil {
		msg = err.Error()
	}
	return []string{
		"",
		"  " + Paint("BLOCK "+msg, TokDanger, caps),
		"",
		"  " + Paint("press u to retry", TokDim, caps),
	}
}

// emptyLines renders the empty state: a calm, non-alarming message. An empty
// result is good news (nothing to clean), so it is never a warning color.
func emptyLines(msg string, caps Capabilities) []string {
	return []string{
		"",
		"  " + Paint(Sigil("check", caps)+" "+msg, TokOK, caps),
	}
}

// listRow is one selectable row in a screen's list: cells already formatted, a
// severity token for its status column, and a selected flag.
type listRow struct {
	cells     []string
	token     Token // severity color for the row's status cell (TokDim = neutral)
	selected  bool
	checked   bool // review-item toggle (Waste); false-safe for other screens
	checkable bool // whether a checkbox is drawn at all; BLOCK rows render "  -  "
}

// renderTable renders a column-aligned, selectable list with a header rule. It
// reuses the primitives' Column/cell budgeting so truncation is exact and the
// grid never wraps (§2.2). The focus marker is a width-1 sigil so the grid holds
// with or without Unicode.
func renderTable(cols []Column, rows []listRow, caps Capabilities, checkbox bool) []string {
	lines := make([]string, 0, len(rows)+2)

	// Header.
	head := make([]string, len(cols))
	for i, c := range cols {
		head[i] = c.cell(c.Title, caps)
	}
	prefixW := 2
	if checkbox {
		prefixW = 4 // "[x] " marker room
	}
	header := strings.Repeat(" ", prefixW) + strings.Join(head, "  ")
	lines = append(lines, Paint(header, TokDim, caps))
	lines = append(lines, Paint(strings.Repeat(Sigil("box-h", caps), visibleWidth(header)), TokDim, caps))

	marker := Sigil("focus-marker", caps)
	for _, row := range rows {
		cells := make([]string, len(cols))
		for ci, c := range cols {
			val := ""
			if ci < len(row.cells) {
				val = row.cells[ci]
			}
			cell := c.cell(val, caps)
			// Colorize the status column (last col by convention) by its token.
			if ci == len(cols)-1 && row.token != TokDim {
				cell = Paint(cell, row.token, caps)
			}
			cells[ci] = cell
		}
		prefix := "  "
		if checkbox {
			switch {
			case !row.checkable:
				// BLOCK rows carry no checkbox — they can never be selected for
				// cleaning, and the empty box makes that unmistakable.
				box := "[-] "
				prefix = box
			case row.checked:
				prefix = "[" + Sigil("check", caps) + "] "
			default:
				prefix = "[ ] "
			}
		}
		line := prefix + strings.Join(cells, "  ")
		if row.selected {
			// Focus marker replaces the leading padding; the whole row goes brand.
			line = marker + line[len([]rune(marker)):]
			line = Paint(line, TokBrand, caps)
		}
		lines = append(lines, line)
	}
	return lines
}

// gauge renders a determinate bar: [####····] pct%. It is motion-free and
// carries a numeric percent so it reads without color (§5). token colors the
// filled portion.
func gauge(pct int, width int, token Token, caps Capabilities) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	if width < 4 {
		width = 4
	}
	fillN := pct * width / 100
	fill := strings.Repeat("#", fillN)
	rest := strings.Repeat("·", width-fillN)
	if !caps.UnicodeLayout {
		rest = strings.Repeat(".", width-fillN)
	}
	bar := "[" + Paint(fill, token, caps) + Paint(rest, TokDim, caps) + "]"
	return fmt.Sprintf("%s %3d%%", bar, pct)
}

// clampSelection keeps sel within [0, n). Empty lists select nothing (-1 read as
// "no selection" by callers, but here we return 0 and callers guard on n==0).
func clampSelection(sel, n int) int {
	if n <= 0 {
		return 0
	}
	if sel < 0 {
		return 0
	}
	if sel >= n {
		return n - 1
	}
	return sel
}

// severityToken maps a diagnose/scan severity integer to a semantic color token.
// 0=ok, 1=attention(warn), 2=critical(danger). Anything else is neutral.
func severityToken(sev int) Token {
	switch sev {
	case 0:
		return TokOK
	case 1:
		return TokWarn
	case 2:
		return TokDanger
	default:
		return TokDim
	}
}

// scanSeverityToken maps a scan finding's severity string to a token. The
// jackal.Severity vocab is safe | caution | warning (there is no "protected"):
// "warning" is a danger BLOCK (data/config that may break — never auto-cleaned),
// "safe" and "caution" are reclaimable (shown warn per proof §6.1).
func scanSeverityToken(sev string) Token {
	switch strings.ToLower(sev) {
	case "warning":
		return TokDanger
	case "caution":
		return TokWarn
	default:
		return TokWarn // safe items are reclaimable — shown warn per proof §6.1
	}
}

// dispatchDone is the generic result of a screen's action dispatch (clean, fix,
// relieve). It carries the report so the screen can show the result inline plus
// a toast; err is set on failure.
type dispatchDone struct {
	kind   string // "clean" | "relieve" | "fix"
	report any
	err    error
}

// runCmd wraps a blocking runner call in a tea.Cmd, delivering a dispatchDone.
// It never blocks the runloop — bubbletea runs the returned func on a goroutine.
func runCmd(kind string, fn func() (any, error)) tea.Cmd {
	return func() tea.Msg {
		r, err := fn()
		return dispatchDone{kind: kind, report: r, err: err}
	}
}
