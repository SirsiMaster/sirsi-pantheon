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

// renderTable renders a column-aligned, selectable list with a header rule,
// showing every row unwindowed. It is for lists whose length is bounded by
// construction (Pulse's --top 5); unbounded selectable lists go through
// renderTableWindow so the cursor can never scroll off-viewport (P2#6).
func renderTable(cols []Column, rows []listRow, caps Capabilities, checkbox bool) []string {
	return renderTableWindow(cols, rows, caps, checkbox, -1)
}

// renderTableWindow renders the table windowed to fit budget terminal lines
// (the 2 header lines + data rows + clip-indicator lines). It reuses the
// primitives' Column/cell budgeting so truncation is exact and the grid never
// wraps (§2.2). The focus marker is a width-1 sigil so the grid holds with or
// without Unicode.
//
// Windowing (P2#6): the selected row is ALWAYS inside the window — the scroll
// offset follows the selection — and rows clipped above/below are declared by
// honest "↑ N more" / "↓ N more" indicator lines (TokDim), never silently
// dropped off the frame. The window is a pure function of the current selection
// and budget, recomputed every render, so it is resize-safe by construction: a
// tea.WindowSizeMsg changes the height the screen passes down and the next
// render re-windows. budget < 0 disables windowing (render everything).
func renderTableWindow(cols []Column, rows []listRow, caps Capabilities, checkbox bool, budget int) []string {
	lines := make([]string, 0, len(rows)+4)

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

	rowBudget := budget
	if rowBudget >= 0 {
		rowBudget -= 2 // the two header lines just drawn
	}
	start, end, above, below := tableWindow(rows, rowBudget)
	if above > 0 {
		lines = append(lines, clipIndicator("arrow-up", above, prefixW, caps))
	}

	marker := Sigil("focus-marker", caps)
	for _, row := range rows[start:end] {
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
	if below > 0 {
		lines = append(lines, clipIndicator("arrow-down", below, prefixW, caps))
	}
	return lines
}

// tableWindow computes the visible row window [start, end) for a slot budget
// (data rows + indicator lines — each shown indicator consumes one slot, so the
// block never exceeds the budget). The selected row is always inside the
// window; above/below are the exact clipped-row counts the indicators declare.
// budget < 0, or a list that fits, disables windowing.
func tableWindow(rows []listRow, budget int) (start, end, above, below int) {
	n := len(rows)
	if budget < 0 || n <= budget {
		return 0, n, 0, 0
	}
	if budget < 3 {
		// Floor: one data row plus up to two indicators. A real frame never gets
		// this tight (the console min-size gates at 80x24 and screens keep their
		// chrome small); the floor just keeps the selection visible if one does —
		// the console clamps any overflow.
		budget = 3
	}
	sel := 0
	for i, r := range rows {
		if r.selected {
			sel = i
			break
		}
	}
	// Selection near the top: nothing clipped above; one slot feeds the
	// below-indicator.
	if k := budget - 1; sel < k {
		return 0, k, 0, n - k
	}
	// Selection near the bottom: nothing clipped below.
	if k := budget - 1; sel >= n-k {
		return n - k, n, n - k, 0
	}
	// Middle: both indicators show; center the selection in the remaining slots.
	k := budget - 2
	start = sel - k/2
	if start < 1 {
		start = 1
	}
	if start > n-k-1 {
		start = n - k - 1
	}
	return start, start + k, start, n - start - k
}

// clipIndicator is the honest "rows are clipped here" line: a direction arrow
// and the exact hidden-row count, dimmed, aligned under the row prefix.
func clipIndicator(arrowSigil string, n, prefixW int, caps Capabilities) string {
	return Paint(strings.Repeat(" ", prefixW)+Sigil(arrowSigil, caps)+fmt.Sprintf(" %d more", n), TokDim, caps)
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
