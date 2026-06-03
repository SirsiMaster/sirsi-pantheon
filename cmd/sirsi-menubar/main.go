// Package main — sirsi-menubar
//
// ☥ Sirsi Menu Bar Application (ADR-010)
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal/rules"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// version is sourced from the shared build-version contract, stamped via ldflags.
var version = modversion.Version

func main() {
	// Version contract: `sirsi-menubar version [--json]`. Lets internal/selfupdate
	// probe this binary for sibling drift (the CTR deploy-drift class, ADR-023).
	if len(os.Args) > 1 && os.Args[1] == "version" {
		info := modversion.Current("sirsi-menubar")
		if len(os.Args) > 2 && os.Args[2] == "--json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(info)
		} else {
			fmt.Printf("☥ Sirsi Menubar %s\n", info.Version)
		}
		return
	}

	unlock, err := platform.TryLock("menubar")
	if err != nil {
		fmt.Printf("☥ Sirsi Menubar is already running. Exiting.\n")
		os.Exit(0)
	}
	defer unlock()

	if os.Getenv("SIRSI_HEADLESS") == "1" {
		runHeadless()
		return
	}

	systray.Run(onReady, onExit)
}

func runHeadless() {
	fmt.Printf("☥ Sirsi Menubar %s (Headless Mode)\n", version)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
}

// menubarNodeStatus produces the Horus local-node read-model for the menubar's
// in-process dashboard (ADR-026 4a) and the 4b ops rows. It reuses the menubar's
// own router-root resolution (which carries the launchd cwd=/ fallback, ADR-021)
// and derives the repo root for router.CollectNodeStatus. Unresolved root →
// error (the dashboard endpoint 503s; the 4b refresh loop skips the update).
func menubarNodeStatus() (*router.NodeStatus, error) {
	routerRoot, ok := resolveRouterRoot()
	if !ok {
		return nil, fmt.Errorf("router root not resolvable from menubar context")
	}
	repoRoot := filepath.Dir(filepath.Dir(routerRoot)) // strip /.agents/idea-router
	return router.CollectNodeStatus(repoRoot, nil)
}

func onReady() {
	systray.SetTemplateIcon(AnkhIcon, AnkhIcon)
	systray.SetTitle("Sirsi")
	systray.SetTooltip("Sirsi Ecosystem Monitor")

	// ── Open TUI ──────────────────────────────────────────────────
	mDashboard := systray.AddMenuItem("Open Console", "Open the full Sirsi console in Terminal")

	// ── Stats section ───────────────────────────────────────────────
	mStats := systray.AddMenuItem("Loading...", "Click to refresh stats")
	systray.AddSeparator()

	// ── Horus ops read-model (ADR-026 step 4b) ─────────────────────
	// One lead row (worst-glyph roll-up, clickable → opens the full dashboard/TUI)
	// + bounded agent rows, refreshed from the same NodeStatus the in-process
	// dashboard serves. Read-only projection — no re-aggregation (Summarize is
	// the canonical reduction). Rows are disabled (informational).
	mOpsHeader := systray.AddMenuItem("ops: loading…", "Horus local-node status — click to open the full dashboard")
	const opsRowCount = 12
	opsRows := make([]*systray.MenuItem, opsRowCount)
	for i := range opsRows {
		opsRows[i] = systray.AddMenuItem("  —", "")
		opsRows[i].Disable()
	}
	systray.AddSeparator()

	// ── Ra section ──────────────────────────────────────────────────
	mRaHeader := systray.AddMenuItem("Agent Fleet", "AI agent orchestration — click for status")
	mRaDeploy := systray.AddMenuItem("  Deploy All Scopes", "sirsi ra deploy")
	mRaKill := systray.AddMenuItem("  Kill All Windows", "sirsi ra kill")
	mRaCollect := systray.AddMenuItem("  Collect Results", "sirsi ra collect")

	// Ra scope status items (updated dynamically, clickable to view logs)
	raScopes := make([]*systray.MenuItem, 4)
	for i := range raScopes {
		raScopes[i] = systray.AddMenuItem("  —", "Click to view scope log")
	}

	systray.AddSeparator()

	// ── Recent Activity ─────────────────────────────────────────────
	mRecentHeader := systray.AddMenuItem("Recent Activity", "Last 5 operations")
	mRecentHeader.Disable()
	recentItems := make([]*systray.MenuItem, 5)
	for i := range recentItems {
		recentItems[i] = systray.AddMenuItem("  —", "")
		recentItems[i].Disable()
	}

	systray.AddSeparator()

	// ── Deity Commands (glyphs match internal/deity/registry.go) ───
	mScan := systray.AddMenuItem("Scan for Waste", "Scan the workstation for reclaimable space and junk")
	mJudge := systray.AddMenuItem("Clean Waste…", "Review and trash waste — opens a confirmation in Terminal")
	mKa := systray.AddMenuItem("Find Leftover Apps", "Detect remnants of uninstalled apps")
	mMaat := systray.AddMenuItem("Quality Audit", "Run the workstation quality + governance audit")
	mGuard := systray.AddMenuItem("Start Watchdog…", "Start the resource watchdog — opens in Terminal")

	systray.AddSeparator()

	// ── More Deities ────────────────────────────────────────────────
	mThoth := systray.AddMenuItem("Sync Memory", "Sync project memory from source + git history")
	mSeshat := systray.AddMenuItem("Ingest Knowledge", "Ingest configured knowledge sources")
	mSeba := systray.AddMenuItem("Hardware Info", "CPU, GPU, and accelerator summary")
	mOsiris := systray.AddMenuItem("Uncommitted Risk", "Assess risk from uncommitted work")
	mNet := systray.AddMenuItem("Consistency Check", "Validate cross-module consistency")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Sirsi", "Exit menubar app")

	// ── Open notification store ─────────────────────────────────────
	nStore, _ := notify.Open(notify.DefaultPath())

	// ── Start dashboard server ──────────────────────────────────────
	cfg := DefaultStatsConfig()
	eventBuf := dashboard.NewEventBuffer(256)
	sirsiBin := findSirsiBinary()
	dashSrv := dashboard.New(dashboard.Config{
		Port:     dashboard.DashboardPort,
		NotifyDB: nStore,
		Events:   eventBuf,
		SirsiBin: sirsiBin,
		StatsFn: func() ([]byte, error) {
			snap := CollectStats(cfg)
			return json.Marshal(snap)
		},
		// ADR-026 step 4 (surface-chrome lane): serve the Horus ops read-model
		// from the menubar's in-process dashboard (GET /api/node-status [+ ?view=
		// summary]). menubarNodeStatus reuses the menubar's own router-root
		// resolution (launchd cwd=/ case, ADR-021); unresolved → error → 503, the
		// designed degrade. Read-only, no destructive surface. The 4b refresh loop
		// reduces the same producer into menu rows (one read-model, one producer).
		NodeStatusFn: menubarNodeStatus,
	})
	if err := dashSrv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "dashboard: %v\n", err)
	}

	// ── Guard watchdog + periodic scan ─────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel // used in quit handler
	startGuardBridge(ctx)
	startPeriodicScan(ctx)

	// ── CTR router registration (A26/A27) ──────────────────────────
	// Register the menubar as a resident, router-visible surface bound to this
	// process PID, with a bounded heartbeat. Best-effort: skipped if no router
	// root is reachable. See register.go.
	menubarRouterRoot, menubarThreadID := registerMenubarThread(ctx)

	// Close the resident thread cleanly on SIGTERM/SIGINT (launchd kickstart,
	// logout, shutdown). systray's event loop below does NOT catch signals, so
	// without this the record would linger active until OS-truth reaping. This
	// is the "close on shutdown if feasible" path; reaping (ADR-022) is fallback.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		cancel()
		closeMenubarThread(menubarRouterRoot, menubarThreadID)
		_ = dashSrv.Stop()
		systray.Quit()
	}()

	// ── Background stats + recent activity loop ─────────────────────
	go func() {
		for {
			snap := CollectStats(cfg)
			lines := snap.FormatMenuItems()
			mStats.SetTitle(lines[0])
			mStats.SetTooltip(snap.StatusLine())

			// Feed RAM pressure into live state for title updates
			liveState.mu.Lock()
			liveState.ramPressure = snap.RAMPressure
			liveState.mu.Unlock()
			liveState.updateTitle()

			// Update Ra scope items.
			for i, item := range raScopes {
				if i < len(snap.RaScopes) {
					s := snap.RaScopes[i]
					item.SetTitle(fmt.Sprintf("  %s %s — %s", s.Icon, s.Name, s.State))
				} else {
					item.SetTitle("  —")
				}
			}

			// Update recent activity items.
			if nStore != nil {
				recent, _ := nStore.Recent(5)
				for i, item := range recentItems {
					if i < len(recent) {
						r := recent[i]
						icon := notify.SeverityIcon(r.Severity)
						item.SetTitle(fmt.Sprintf("  %s %s — %s", icon, r.Source, r.Summary))
					} else {
						item.SetTitle("  —")
					}
				}
			}

			// Update Horus ops read-model rows (ADR-026 step 4b). Reduce the same
			// NodeStatus the in-process dashboard serves into the bounded summary,
			// then render via the unit-tested opsLeadRow/opsAgentRows. Best-effort:
			// an unresolved root / collect error leaves the prior titles in place.
			if ns, err := menubarNodeStatus(); err == nil && ns != nil {
				sum := dashboard.Summarize(ns, opsRowCount)
				mOpsHeader.SetTitle(opsLeadRow(sum))
				rows := opsAgentRows(sum)
				for i, item := range opsRows {
					if i < len(rows) {
						item.SetTitle(rows[i])
					} else {
						item.SetTitle("  —")
					}
				}
			}

			time.Sleep(cfg.Interval)
		}
	}()

	// ── Event loop — all user actions route through the TUI (ADR-016) ──
	for {
		select {
		case <-mDashboard.ClickedCh:
			spawnTUIWindow()
		case <-mOpsHeader.ClickedCh:
			spawnTUIWindow() // ADR-026 4b: lead ops row opens the full dashboard/TUI
		case <-mStats.ClickedCh:
			spawnTUIWithCommand("diagnose")
		case <-mRaHeader.ClickedCh:
			spawnTUIWithCommand("ra status")
		case <-mRaDeploy.ClickedCh:
			spawnTUIWithCommand("ra deploy")
		case <-mRaKill.ClickedCh:
			spawnTUIWithCommand("ra kill")
		case <-mRaCollect.ClickedCh:
			runActionInPlace(mRaCollect, "  Collect Results", sirsiBin, "ra collect", nStore)
		case <-raScopes[0].ClickedCh:
			spawnTUIWithCommand("ra status")
		case <-raScopes[1].ClickedCh:
			spawnTUIWithCommand("ra status")
		case <-raScopes[2].ClickedCh:
			spawnTUIWithCommand("ra status")
		case <-raScopes[3].ClickedCh:
			spawnTUIWithCommand("ra status")
		// Anubis — scan/ghosts are read-only → execute in place; clean DELETES →
		// keep the Terminal/confirm path (Rule A1: no one-click destruction).
		case <-mScan.ClickedCh:
			runActionInPlace(mScan, "Scan for Waste", sirsiBin, "scan", nStore)
		case <-mJudge.ClickedCh:
			spawnTUIWithCommand("clean") // destructive — confirm in Terminal
		case <-mKa.ClickedCh:
			runActionInPlace(mKa, "Find Leftover Apps", sirsiBin, "ghosts", nStore)
		case <-mMaat.ClickedCh:
			runActionInPlace(mMaat, "Quality Audit", sirsiBin, "maat audit", nStore)
		// Isis
		case <-mGuard.ClickedCh:
			spawnTUIWithCommand("guard")
		// Additional deities — safe (non-destructive) actions execute in place
		// and report into Recent Activity; no Terminal window (user complaint fix).
		case <-mThoth.ClickedCh:
			runActionInPlace(mThoth, "Sync Memory", sirsiBin, "thoth sync", nStore)
		case <-mSeshat.ClickedCh:
			runActionInPlace(mSeshat, "Ingest Knowledge", sirsiBin, "seshat ingest", nStore)
		case <-mSeba.ClickedCh:
			runActionInPlace(mSeba, "Hardware Info", sirsiBin, "seba hardware", nStore)
		case <-mOsiris.ClickedCh:
			runActionInPlace(mOsiris, "Uncommitted Risk", sirsiBin, "osiris risk", nStore)
		case <-mNet.ClickedCh:
			runActionInPlace(mNet, "Consistency Check", sirsiBin, "net align", nStore)
		case <-mQuit.ClickedCh:
			cancel()
			closeMenubarThread(menubarRouterRoot, menubarThreadID)
			_ = dashSrv.Stop()
			if nStore != nil {
				nStore.Close()
			}
			systray.Quit()
			return
		}
	}
}

func onExit() {}

// ── TUI Bridge ─────────────────────────────────────────────────────────

// spawnTUIWindow opens Terminal.app (or iTerm2) running `sirsi` which
// launches the BubbleTea TUI. Uses the same AppleScript pattern as
// ra.SpawnWindow but without the agent machinery.
// spawnTUIWindow opens the TUI with no pre-loaded command.
func spawnTUIWindow() {
	spawnTUIWithCommand("")
}

// spawnTUIWithCommand opens or activates a Sirsi terminal window and runs a
// concrete CLI command. The older ADR-016 bridge expected a `sirsi pantheon`
// TUI command, but the active CLI surface exposes direct commands instead.
//
// This is the ONLY way the menubar should interact with the user —
// everything happens inside the TUI viewport (ADR-016).
func spawnTUIWithCommand(command string) {
	sirsiBin := findSirsiBinary()
	commandLine := shellQuote(sirsiBin)
	if command != "" {
		commandLine += " " + command
	} else {
		commandLine += " status"
	}
	commandLine += "; echo; read -n 1 -s -r '?Press any key to close...'"

	// Check if iTerm2 is installed, prefer it over Terminal.app
	if _, err := os.Stat("/Applications/iTerm.app"); err == nil {
		// Try to find existing TUI window first
		script := fmt.Sprintf(`tell application "iTerm"
	activate
	-- Look for existing Sirsi TUI window
	set foundSession to false
	repeat with aWindow in windows
		repeat with aTab in tabs of aWindow
			repeat with aSession in sessions of aTab
				if name of aSession contains "Sirsi" or name of aSession contains "sirsi" then
					select aSession
					set foundSession to true
					exit repeat
				end if
			end repeat
			if foundSession then exit repeat
		end repeat
		if foundSession then exit repeat
	end repeat
	if not foundSession then
		set newWindow to (create window with default profile)
		tell current session of newWindow
			write text "%s"
			set name to "☥ Sirsi"
		end tell
	else
		tell current session of current window
			write text "%s"
		end tell
	end if
end tell`, escapeAppleScript(commandLine), escapeAppleScript(commandLine))
		runAppleScript("iterm", script)
		return
	}

	// Terminal.app fallback — check for existing window, create if needed
	script := fmt.Sprintf(`
-- Check if a Sirsi TUI window already exists
tell application "Terminal"
	set foundWindow to false
	repeat with w in windows
		if custom title of w is "☥ Sirsi" or name of w contains "sirsi" then
			set frontmost of w to true
			activate
			set foundWindow to true
			exit repeat
		end if
	end repeat
	if not foundWindow then
		activate
		do script "%s"
		delay 0.5
		set custom title of front window to "☥ Sirsi"
	else
		do script "%s" in front window
	end if
end tell`, escapeAppleScript(commandLine), escapeAppleScript(commandLine))
	runAppleScript("terminal", script)
}

// escapeAppleScript escapes backslashes and double quotes for AppleScript strings.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	return s
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func runAppleScript(label, script string) {
	go func() {
		cmd := exec.Command("osascript", "-e", script)
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "menubar %s launch failed: %v\n%s\n", label, err, strings.TrimSpace(string(out)))
		}
	}()
}

// ── Live State ─────────────────────────────────────────────────────────

// menubarState tracks the current state for the menubar title.
type menubarState struct {
	mu           sync.RWMutex
	wasteBytes   int64
	wasteLabel   string
	ramPressure  string // "low", "medium", "high"
	guardAlert   string // latest guard alert process name, or ""
	guardAlertAt time.Time
}

var liveState = &menubarState{}

// updateTitle sets the menubar title based on the current live state.
// Priority: guard alert (if recent) > RAM pressure > waste > clean.
func (s *menubarState) updateTitle() {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Guard alert takes priority if within last 5 minutes
	if s.guardAlert != "" && time.Since(s.guardAlertAt) < 5*time.Minute {
		systray.SetTitle("⚠️ " + s.guardAlert)
		systray.SetTooltip(fmt.Sprintf("Process alert: %s", s.guardAlert))
		return
	}

	// High RAM pressure
	if s.ramPressure == "high" {
		systray.SetTitle("🔴 RAM")
		systray.SetTooltip("High RAM pressure detected")
		return
	}

	// Waste found (> 1 GB)
	if s.wasteBytes > 1<<30 {
		systray.SetTitle("🟡 " + s.wasteLabel)
		systray.SetTooltip(fmt.Sprintf("Infrastructure waste: %s", s.wasteLabel))
		return
	}

	// All clean
	systray.SetTitle("🟢 Clean")
	systray.SetTooltip("Sirsi Ecosystem Monitor — all clean")
}

// startGuardBridge starts the guard watchdog and pipes alerts into live state.
func startGuardBridge(ctx context.Context) {
	cfg := guard.DefaultBridgeConfig()
	cfg.WatchConfig.AutoRenice = true
	cfg.OnAlert = func(alert guard.AlertEntry) {
		liveState.mu.Lock()
		liveState.guardAlert = alert.ProcessName
		liveState.guardAlertAt = time.Now()
		liveState.mu.Unlock()
		liveState.updateTitle()
	}
	_ = guard.StartBridge(ctx, cfg)
}

// startPeriodicScan runs a jackal scan on first launch, then every 4 hours.
// Persists findings to disk and updates the menubar title.
func startPeriodicScan(ctx context.Context) {
	go func() {
		for {
			engine := jackal.DefaultEngine()
			engine.RegisterAll(rules.AllRules()...)
			start := time.Now()
			res, err := engine.Scan(ctx, jackal.ScanOptions{})
			if err == nil {
				jackal.EnrichAdvisory(res)
				_ = jackal.Persist(res, time.Since(start))

				liveState.mu.Lock()
				liveState.wasteBytes = res.TotalSize
				liveState.wasteLabel = jackal.FormatSize(res.TotalSize) + " waste"
				liveState.mu.Unlock()
				liveState.updateTitle()
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(4 * time.Hour):
			}
		}
	}()
}

// AnkhIcon is the menu bar icon data, generated by the Ankh renderer.
var AnkhIcon = getIcon()
