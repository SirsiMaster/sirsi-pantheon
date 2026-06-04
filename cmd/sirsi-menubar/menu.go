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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"github.com/SirsiMaster/sirsi-pantheon/internal/workstream"
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

// ── About / Surfaces ────────────────────────────────────────────────────────

// claudeMCPLinked reports whether the Sirsi MCP server is registered in the
// user's Claude config (~/.claude.json) — i.e. Claude Code can actually call
// Sirsi tools. Honest "linked" vs merely "available" (Rule A23/A14).
func claudeMCPLinked() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	for _, p := range []string{
		filepath.Join(home, ".claude.json"),
		filepath.Join(home, ".config", "claude", "mcp.json"),
		filepath.Join(home, ".cursor", "mcp.json"),
	} {
		if data, err := os.ReadFile(p); err == nil && strings.Contains(string(data), "\"sirsi\"") {
			return true
		}
	}
	return false
}

// appSearchDirs is the set of macOS application directories. Variable for tests.
var appSearchDirs = func() []string {
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	return dirs
}

// ideAppNames maps a launcher tool ID to its macOS .app bundle name.
var ideAppNames = map[string]string{
	"vscode":   "Visual Studio Code",
	"code":     "Visual Studio Code",
	"cursor":   "Cursor",
	"zed":      "Zed",
	"windsurf": "Windsurf",
}

// ideAppInstalled detects a GUI IDE install that has no CLI shim on PATH (e.g.
// VS Code without the `code` command). Without this, ScanInventory's LookPath-
// only check shows a false ✗ for an IDE the user clearly has — About must not
// misrepresent the surfaces (Rule A23/A14).
func ideAppInstalled(toolID string) bool {
	name, ok := ideAppNames[toolID]
	if !ok {
		return false
	}
	for _, base := range appSearchDirs() {
		if _, err := os.Stat(filepath.Join(base, name+".app")); err == nil {
			return true
		}
	}
	return false
}

// addAboutSection builds the "About Pantheon" submenu: version + every surface
// Pantheon is installed/linked in (this menubar, the CLI, AI assistants, IDEs,
// the MCP bridge to Claude Code). The TUI surface view, brought to the menubar.
// Every actionable row leads somewhere (Configure, MCP setup); status rows are
// informational. Built once at onReady — installed-tool state is effectively
// static for a session.
func addAboutSection(sirsiBin string) {
	about := systray.AddMenuItem("ℹ About Pantheon", "Version, surfaces, and integrations")

	ver := modversion.Current("sirsi-menubar").Version
	about.AddSubMenuItem("𓉴 Sirsi Pantheon "+ver, "Infrastructure hygiene + agent orchestration").Disable()
	about.AddSubMenuItem("──  Surfaces & Integrations  ──", "Where Pantheon is installed and linked").Disable()

	// The running surface.
	about.AddSubMenuItem("  𓂀 Menubar — active (this app)", "The Sirsi menubar is running").Disable()
	if sirsiBin != "" {
		about.AddSubMenuItem("  ⌘ CLI — "+sirsiBin, "The sirsi command-line tool").Disable()
	}

	// AI assistants + IDEs detected on this machine.
	inv := workstream.ScanInventory(platform.Current())
	for _, t := range inv.Tools {
		installed := t.Installed
		if !installed && t.Kind == "ide" {
			installed = ideAppInstalled(t.ID) // catch GUI installs with no CLI on PATH
		}
		icon, state := "✗", "not installed"
		if installed {
			icon, state = "✓", "installed"
		}
		kind := "AI"
		if t.Kind == "ide" {
			kind = "IDE"
		}
		row := about.AddSubMenuItem(fmt.Sprintf("  %s %s — %s (%s)", icon, t.Name, state, kind), t.Name+" "+kind)
		row.Disable() // status row; the actionable path is Configure Integrations below
	}

	// MCP bridge — the Claude Code / IDE link. Honest linked vs available.
	mcpTitle := "  ↳ MCP bridge — available (sirsi mcp)"
	mcpTip := "Start the MCP server so an IDE can call Sirsi tools — click to configure"
	if claudeMCPLinked() {
		mcpTitle = "  ↳ MCP bridge — ✓ linked to Claude Code"
		mcpTip = "Sirsi is registered in your Claude/Cursor MCP config"
	}
	mcpRow := about.AddSubMenuItem(mcpTitle, mcpTip)

	// Actionable: configure/link integrations (sirsi setup).
	cfgRow := about.AddSubMenuItem("  ⚙ Configure Integrations…", "Install / link AI assistants, IDEs, and the MCP bridge")
	wire(mcpRow, func() { spawnTUIWithCommand("setup") })
	wire(cfgRow, func() { spawnTUIWithCommand("setup") })
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
	lastHyphen := false
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == ' ', r == '-', r == '_':
			if !lastHyphen { // collapse runs of separators (incl. dropped glyphs/em-dashes)
				b.WriteByte('-')
				lastHyphen = true
			}
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
