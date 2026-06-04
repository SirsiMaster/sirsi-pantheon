package main

// menu.go — the carrot/submenu UX layer (2026-06-04).
//
// The menubar is organized into collapsible deity sections (native macOS
// submenus). Every service is clickable and runs a complete activity; every
// action produces a visible event response that is itself clickable to read the
// full discovery detail. No dead ends:
//
//	deity ▸                       (AddSubMenuItem — native caret, expands on click)
//	  service          → runs the action, gives ⏳→✓/✗ feedback
//	  ↳ last result     → the event response; click to open the full detail
//
// This file owns the reusable primitives (wire, resultRow, runService,
// openDetail, recent-activity drill-in). main.go composes them in onReady.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
)

// wire runs handler on every click of item, each in its own goroutine. This
// replaces a monolithic select over dozens of channels — submenus can grow
// without a central dispatch block (Rule 0). A disabled item emits no clicks,
// so wiring a not-yet-active result row is safe.
func wire(item *systray.MenuItem, handler func()) {
	if item == nil {
		return
	}
	go func() {
		for range item.ClickedCh {
			handler()
		}
	}()
}

// resultRow is a deity's inline "↳ last result" item. After an action runs it
// shows the event response (icon + summary) and, when clicked, opens the full
// discovery detail. This is what makes every action lead to a complete,
// drillable activity.
type resultRow struct {
	item    *systray.MenuItem
	mu      sync.Mutex
	title   string
	details string
}

// newDeityResult adds the "↳ last result" child under a deity submenu parent and
// wires its click to open the stored detail. Starts disabled (no activity yet).
func newDeityResult(parent *systray.MenuItem) *resultRow {
	it := parent.AddSubMenuItem("  ↳ (no activity yet)", "The latest result appears here — click to read the full detail")
	it.Disable()
	r := &resultRow{item: it}
	wire(it, r.open)
	return r
}

func (r *resultRow) set(title, icon, summary, details string) {
	r.mu.Lock()
	r.title, r.details = title, details
	r.mu.Unlock()
	r.item.SetTitle("  ↳ " + icon + " " + truncate(summary, 56))
	r.item.SetTooltip("Click to read the full result")
	r.item.Enable()
}

func (r *resultRow) open() {
	r.mu.Lock()
	t, d := r.title, r.details
	r.mu.Unlock()
	if d == "" {
		return
	}
	openDetail(t, d)
}

// runService executes `sirsi <command>` off the UI thread, gives the clicked
// item ⏳→✓/✗ feedback, records the response to Recent Activity, and fills the
// deity's result row (clickable → full detail). Non-blocking; the menu stays
// responsive. This is the safe (non-destructive) action runner — destructive
// flows (clean/ra deploy/kill) keep their dedicated confirm/Terminal paths.
func runService(clicked *systray.MenuItem, label, sirsiBin, command string, store *notify.Store, rr *resultRow) {
	if clicked == nil || sirsiBin == "" {
		return
	}
	clicked.SetTitle("⏳ " + label)
	clicked.Disable()
	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), actionTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, sirsiBin, strings.Fields(command)...).CombinedOutput()
		dur := time.Since(start)
		text := stripANSI(string(out))

		sev, icon := notify.SeveritySuccess, "✓"
		summary := firstMeaningfulLine(text)
		if ctx.Err() == context.DeadlineExceeded {
			sev, icon, summary = notify.SeverityWarning, "⏱", "timed out after "+actionTimeout.String()
		} else if err != nil {
			sev, icon = notify.SeverityError, "✗"
			if summary == "" {
				summary = err.Error()
			}
		}
		if summary == "" {
			summary = "done"
		}

		if store != nil {
			_ = store.Record(notify.Notification{
				Source:     label,
				Action:     command,
				Severity:   sev,
				Summary:    truncate(summary, 140),
				Details:    truncate(text, 8000),
				DurationMs: dur.Milliseconds(),
			})
		}
		if rr != nil {
			rr.set(label, icon, summary, text)
		}
		clicked.SetTitle(icon + " " + label)
		clicked.Enable()
		time.AfterFunc(5*time.Second, func() { clicked.SetTitle(label) })
	}()
}

// ── Recent Activity drill-in ───────────────────────────────────────────────

var (
	recentMu   sync.Mutex
	recentSnap []notify.Notification
)

// setRecentSnap stores the latest Recent Activity notifications so the wired
// recent-item clicks can open the matching detail. Called by the stats loop.
func setRecentSnap(ns []notify.Notification) {
	recentMu.Lock()
	recentSnap = ns
	recentMu.Unlock()
}

// openRecentDetail opens the full detail for the i-th Recent Activity row.
func openRecentDetail(i int) {
	recentMu.Lock()
	var n notify.Notification
	ok := i >= 0 && i < len(recentSnap)
	if ok {
		n = recentSnap[i]
	}
	recentMu.Unlock()
	if !ok {
		return
	}
	openDetail(n.Source+" — "+n.Summary, n.Details)
}

// ── Detail viewer ───────────────────────────────────────────────────────────

// openDetail writes a result's full discovery to a temp file and opens it in the
// default viewer — the "click the event to read the detail" leaf. A plain file +
// `open` shows the complete output with no truncation and needs no extra macOS
// permission (unlike a Finder/Automation prompt).
func openDetail(title, body string) {
	if strings.TrimSpace(body) == "" {
		body = "(no output captured)"
	}
	dir := filepath.Join(os.TempDir(), "sirsi-details")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, slugify(title)+".txt")
	header := "═══════════════════════════════════════════\n  " + title + "\n═══════════════════════════════════════════\n\n"
	if err := os.WriteFile(path, []byte(header+body+"\n"), 0o644); err != nil {
		return
	}
	_ = exec.Command("open", path).Start()
}

// slugify reduces a title to a stable filename stem.
func slugify(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ', r == '-', r == '_':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "result"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
