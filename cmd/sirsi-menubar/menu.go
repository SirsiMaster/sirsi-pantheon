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
// This file owns the reusable primitives (wire, resultRow,
// openDetail, recent-activity drill-in). main.go composes them in onReady.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"

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

// mcpEntry is one MCP server registration in an IDE config.
type mcpEntry struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

// hasValidSirsiServer reports whether a server map registers the Sirsi MCP server
// PROPERLY: command is `sirsi` (or a path ending /sirsi) AND args include "mcp".
// No string-search shortcuts — a stray "sirsi" elsewhere must not read as linked
// (Rule A23/A14: never overclaim an integration).
func hasValidSirsiServer(servers map[string]mcpEntry) bool {
	s, ok := servers["sirsi"]
	if !ok {
		return false
	}
	if s.Command != "sirsi" && !strings.HasSuffix(s.Command, "/sirsi") {
		return false
	}
	for _, a := range s.Args {
		if a == "mcp" {
			return true
		}
	}
	return false
}

// mcpLinkedInConfig parses an IDE MCP config file and reports whether Sirsi is a
// valid registered server — checking both the top-level mcpServers and any
// per-project mcpServers block (Claude Code stores servers per project).
func mcpLinkedInConfig(path string) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg struct {
		MCPServers map[string]mcpEntry `json:"mcpServers"`
		Projects   map[string]struct {
			MCPServers map[string]mcpEntry `json:"mcpServers"`
		} `json:"projects"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	if hasValidSirsiServer(cfg.MCPServers) {
		return true
	}
	for _, p := range cfg.Projects {
		if hasValidSirsiServer(p.MCPServers) {
			return true
		}
	}
	return false
}

// claudeMCPLinked / cursorMCPLinked report per-client linkage from the client's
// own config — never conflating one client's config as proof for another.
func claudeMCPLinked() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return mcpLinkedInConfig(filepath.Join(home, ".claude.json")) ||
		mcpLinkedInConfig(filepath.Join(home, ".config", "claude", "mcp.json"))
}

func cursorMCPLinked() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	return mcpLinkedInConfig(filepath.Join(home, ".cursor", "mcp.json"))
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

// addMCPClientRow adds one IDE's MCP-linkage status row — honest ✓ linked / ○
// not linked, per that client's own config.
func addMCPClientRow(about *systray.MenuItem, label string, linked bool) {
	icon, state := "○", "not linked"
	if linked {
		icon, state = "✓", "linked"
	}
	about.AddSubMenuItem("  ↳ "+label+" — "+icon+" "+state, label+" registration of the Sirsi MCP server")
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
	about.AddSubMenuItem("  𓂀 Menubar — active (this app)", "The Sirsi menubar is running")
	if sirsiBin != "" {
		about.AddSubMenuItem("  ⌘ CLI — "+sirsiBin, "The sirsi command-line tool")
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
		// Status row — left enabled so live install-state renders crisp, not greyed
		// out (a true listing shouldn't look dead). fyne drops clicks on unwired
		// items via a non-blocking send, so it's inert without being disabled. The
		// actionable path is Configure Integrations below.
		about.AddSubMenuItem(fmt.Sprintf("  %s %s — %s (%s)", icon, t.Name, state, kind), t.Name+" "+kind)
	}

	// MCP bridge — reported PER CLIENT, never conflated (A23/A14). The server is
	// "available" because sirsi ships the `mcp` subcommand; each IDE is "linked"
	// only if ITS OWN config registers the sirsi server properly.
	about.AddSubMenuItem("  ↳ MCP server — available (sirsi mcp)", "Sirsi exposes its tools over MCP")
	addMCPClientRow(about, "Claude Code MCP", claudeMCPLinked())
	addMCPClientRow(about, "Cursor MCP", cursorMCPLinked())

	// Actionable: configure/link integrations (sirsi setup).
	cfgRow := about.AddSubMenuItem("  ⚙ Configure Integrations…", "Install / link AI assistants, IDEs, and the MCP bridge")
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
