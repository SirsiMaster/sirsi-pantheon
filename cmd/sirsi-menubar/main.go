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
	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	modversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
	"github.com/fsnotify/fsnotify"
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

// applyFDAState renders the disk-access item to the current all/some/none tier:
// hidden at full visibility, a partial-access nudge when only some folders are
// granted, a blunt no-access warning when blind. Re-evaluated each refresh so it
// disappears the moment the user grants access.
func applyFDAState(item *systray.MenuItem) {
	switch platform.CheckDiskAccess().Level {
	case platform.AccessFull:
		item.Hide()
	case platform.AccessSome:
		item.SetTitle("◐ Partial Visibility — See Everything…")
		item.Show()
	default:
		item.SetTitle("⚠ No Disk Access — Grant Access…")
		item.Show()
	}
}

// maybeFirstRunSetup launches the setup wizard the first time the menubar runs
// on a machine, so a DMG/GUI install drives the same surface + permission
// wizard as `sirsi setup` on the CLI. A marker file makes it strictly one-time;
// the menubar's Configure row re-runs setup on demand thereafter.
func maybeFirstRunSetup() {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	marker := filepath.Join(home, ".config", "sirsi", ".setup-launched")
	if _, err := os.Stat(marker); err == nil {
		return // already run once
	}
	if err := os.MkdirAll(filepath.Dir(marker), 0o755); err != nil {
		return
	}
	// Write the marker first so a failed/closed wizard never loops the prompt.
	if err := os.WriteFile(marker, []byte("1\n"), 0o644); err != nil {
		return
	}
	spawnTUIWithCommand("setup")
}

func onReady() {
	systray.SetTemplateIcon(AnkhIcon, AnkhIcon)
	systray.SetTitle("Sirsi")
	systray.SetTooltip("Sirsi Ecosystem Monitor")

	sirsiBin := findSirsiBinary()
	nStore, _ := notify.Open(notify.DefaultPath())

	// First launch after a DMG/GUI install drives the same surface + permission
	// wizard as the CLI, so "the GUI install implements this" is literal.
	maybeFirstRunSetup()

	// ── Infrastructure: dashboard server, guard, periodic scan, router ──────
	// Set up BEFORE building the menu so the wired click handlers (closures
	// below) can capture ctx/cancel/dashSrv/router cleanly.
	cfg := DefaultStatsConfig()
	eventBuf := dashboard.NewEventBuffer(256)
	dashSrv := dashboard.New(dashboard.Config{
		Port:     dashboard.DashboardPort,
		NotifyDB: nStore,
		Events:   eventBuf,
		SirsiBin: sirsiBin,
		StatsFn: func() ([]byte, error) {
			snap := CollectStats(cfg)
			return json.Marshal(snap)
		},
		// ADR-026 step 4: serve the Horus ops read-model from the in-process
		// dashboard. Unresolved root → error → 503 (designed degrade).
		NodeStatusFn: menubarNodeStatus,
	})
	if err := dashSrv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "dashboard: %v\n", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	startGuardBridge(ctx)
	startPeriodicScan(ctx)
	startLiveRefresh(ctx)
	menubarRouterRoot, menubarThreadID := registerMenubarThread(ctx)

	quit := func() {
		cancel()
		closeMenubarThread(menubarRouterRoot, menubarThreadID)
		_ = dashSrv.Stop()
		if nStore != nil {
			nStore.Close()
		}
		systray.Quit()
	}

	// Close the resident thread cleanly on SIGTERM/SIGINT (launchd kickstart,
	// logout, shutdown). Reaping (ADR-022) is the fallback if this is missed.
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		<-sigCh
		quit()
	}()

	// ── Command Deck — local intelligence control plane ─────────────────────
	mDeck := systray.AddMenuItem("☥ Command Deck", "Live local AI, compute, router, context, and risk")
	const deckRowCount = 5
	deckRows := make([]*systray.MenuItem, deckRowCount)
	for i := range deckRows {
		deckRows[i] = mDeck.AddSubMenuItem("  —", "Live Sirsi control-plane signal")
		deckRows[i].Disable()
	}
	mAskSirsi := mDeck.AddSubMenuItem("Ask Sirsi Local AI…", "Ask the on-device Gemma model for a status read")
	mGemmaServe := mDeck.AddSubMenuItem("Start / Check Gemma Broker…", "Run the warm local Gemma broker")
	mRouterDoctor := mDeck.AddSubMenuItem("Router Doctor…", "Inspect and repair safe router liveness issues")
	mComputeProfile := mDeck.AddSubMenuItem("Compute Profile…", "Inspect Apple Silicon, GPU, and Neural Engine lanes")
	wire(mAskSirsi, func() {
		spawnTUIWithCommand("gemma --task analyze \"Report Sirsi local AI status and the next operator action.\"")
	})
	wire(mGemmaServe, func() { spawnTUIWithCommand("gemma serve --status") })
	wire(mRouterDoctor, func() { spawnTUIWithCommand("router doctor") })
	wire(mComputeProfile, func() { spawnTUIWithCommand("seba compute") })

	// ── Status header (top level) ───────────────────────────────────────────
	mStats := systray.AddMenuItem("Vitals loading…", "Workstation status — click for full diagnostics in Terminal")
	wire(mStats, func() { spawnTUIWithCommand("diagnose") })
	// Foundational visibility: without macOS Full Disk Access, Sirsi/Horus is
	// blind to Desktop/Documents/Mail/app containers. Shown only when missing.
	mFDA := systray.AddMenuItem("⚠ Grant Full Disk Access…", "Grant disk access so Sirsi can see and clean the whole workstation")
	applyFDAState(mFDA)
	wire(mFDA, func() { openFullDiskAccessPane(nStore) })
	systray.AddSeparator()

	// ── 𓂀 Horus — Ops ───────────────────────────────────────────────────────
	// Local-node read-model (ADR-026 4b): lead roll-up + bounded agent rows,
	// reduced from the same NodeStatus the dashboard serves. Rows are
	// informational; the lead + "Open Dashboard" open the full console.
	mHorus := systray.AddMenuItem("𓂀 Horus — Ops", "Local-node operations overview")
	mOpsHeader := mHorus.AddSubMenuItem("ops: loading…", "Horus local-node status")
	const opsRowCount = 12
	opsRows := make([]*systray.MenuItem, opsRowCount)
	for i := range opsRows {
		opsRows[i] = mHorus.AddSubMenuItem("  —", "")
		opsRows[i].Disable()
	}
	mDashboard := mHorus.AddSubMenuItem("↳ Open Full Dashboard…", "Open the full Sirsi console in Terminal")
	wire(mOpsHeader, spawnTUIWindow)
	wire(mDashboard, spawnTUIWindow)

	// ── 𓆄 Ma'at — Quality ────────────────────────────────────────────────────
	mMaatTop := systray.AddMenuItem("𓆄 Ma'at — Quality", "Quality + governance audit, system diagnostics")
	rrMaat := newDeityResult(mMaatTop)
	mMaat := mMaatTop.AddSubMenuItem("Quality Audit", "Run the workstation quality + governance audit")
	mDiag := mMaatTop.AddSubMenuItem("System Diagnostics", "Run sirsi diagnose — full health checks")
	wire(mMaat, func() { runService(mMaat, "Quality Audit", sirsiBin, "maat audit", nStore, rrMaat) })
	wire(mDiag, func() { runService(mDiag, "System Diagnostics", sirsiBin, "diagnose", nStore, rrMaat) })

	// ── 𓁢 Thoth — Memory ─────────────────────────────────────────────────────
	mThothTop := systray.AddMenuItem("𓁢 Thoth — Memory", "Project memory + knowledge ingestion")
	rrThoth := newDeityResult(mThothTop)
	mThoth := mThothTop.AddSubMenuItem("Sync Memory", "Sync project memory from source + git history")
	mSeshat := mThothTop.AddSubMenuItem("Ingest Knowledge", "Ingest configured knowledge sources")
	wire(mThoth, func() { runService(mThoth, "Sync Memory", sirsiBin, "thoth sync", nStore, rrThoth) })
	wire(mSeshat, func() { runService(mSeshat, "Ingest Knowledge", sirsiBin, "seshat ingest", nStore, rrThoth) })

	// ── 𓇶 Ra — Agent Fleet ───────────────────────────────────────────────────
	// Deploy/Kill spawn/terminate windows → Terminal path (Rule A1: no silent
	// one-click fleet mutation). Collect is read-only → in-place.
	mRa := systray.AddMenuItem("𓇶 Ra — Agent Fleet", "AI agent orchestration")
	rrRa := newDeityResult(mRa)
	mRaDeploy := mRa.AddSubMenuItem("Deploy All Scopes", "sirsi ra deploy")
	mRaKill := mRa.AddSubMenuItem("Kill All Windows", "sirsi ra kill")
	mRaCollect := mRa.AddSubMenuItem("Collect Results", "sirsi ra collect")
	raScopes := make([]*systray.MenuItem, 4)
	for i := range raScopes {
		raScopes[i] = mRa.AddSubMenuItem("  —", "Click to view scope status")
		raScopes[i].Disable()
	}
	wire(mRaDeploy, func() { spawnTUIWithCommand("ra deploy") })
	wire(mRaKill, func() { spawnTUIWithCommand("ra kill") })
	wire(mRaCollect, func() { runService(mRaCollect, "Collect Results", sirsiBin, "ra collect", nStore, rrRa) })

	// ── 𓋹 Insight — Hardware / Risk / Consistency ────────────────────────────
	mInsight := systray.AddMenuItem("𓋹 Insight", "Hardware, uncommitted risk, and consistency")
	rrInsight := newDeityResult(mInsight)
	mSeba := mInsight.AddSubMenuItem("Hardware Info", "CPU, GPU, and accelerator summary")
	mOsiris := mInsight.AddSubMenuItem("Uncommitted Risk", "Assess risk from uncommitted work")
	mNet := mInsight.AddSubMenuItem("Consistency Check", "Validate cross-module consistency")
	wire(mSeba, func() { runService(mSeba, "Hardware Info", sirsiBin, "seba hardware", nStore, rrInsight) })
	wire(mOsiris, func() { runService(mOsiris, "Uncommitted Risk", sirsiBin, "osiris risk", nStore, rrInsight) })
	wire(mNet, func() { runService(mNet, "Consistency Check", sirsiBin, "net align", nStore, rrInsight) })

	// ── 📋 Ledger — universal task board (ADR-050/051) ──────────────────────────
	// Same board the owner sees in CLI (`sirsi router ledger`) and MCP
	// (`router_ledger`). Completion %, done/review/queued/blocked counts,
	// and blocked items waiting on owner.
	const ledgerRowCount = 5
	mLedger := systray.AddMenuItem("📋 Ledger — loading…", "Universal task ledger board: completion %, done/queued/blocked")
	mLedgerProgress := mLedger.AddSubMenuItem("  Progress: loading…", "Overall completion %")
	mLedgerProgress.Disable()
	ledgerRows := make([]*systray.MenuItem, ledgerRowCount)
	for i := range ledgerRows {
		ledgerRows[i] = mLedger.AddSubMenuItem("  —", "")
		ledgerRows[i].Disable()
	}
	mLedgerOpen := mLedger.AddSubMenuItem("View full ledger…", "Run sirsi router ledger in Terminal")
	wire(mLedgerOpen, func() { runService(mLedgerOpen, "Ledger", sirsiBin, "router ledger", nStore, nil) })

	// ── 🐺 Anubis — Cleanup (secondary: storage upkeep lives BELOW the live view) ─
	// Memory is the pre-eminent view; disk cleanup is demoted here, plain-English.
	mAnubis := systray.AddMenuItem("🐺 Anubis — Cleanup", "Find and clear files you don't need")
	rrAnubis := newDeityResult(mAnubis)
	mScan := mAnubis.AddSubMenuItem("Find stuff to clear", "Look for files you can safely remove")
	mJudge := mAnubis.AddSubMenuItem("Clear stuff…", "Preview what's safe to remove, then confirm")
	mReview := mAnubis.AddSubMenuItem("  ↳ See exactly what will be removed…", "Review the full list before anything moves")
	mCleanConfirm := mAnubis.AddSubMenuItem("  ✓ Move it to Trash", "Move the previewed items to Trash (you can undo)")
	mCleanConfirm.Hide()
	mKa := mAnubis.AddSubMenuItem("Find leftover app files", "Find bits left behind by apps you deleted")
	mGuard := mAnubis.AddSubMenuItem("Watch for problems…", "Keep an eye on apps using too much")
	// ── Permanent-delete path: preview Trash → confirm → permanently deleted ──
	// Distinct from the clean→trash flow (Rule A1): this is irreversible. The
	// preview item checks the current Trash and arms the confirm only when
	// non-empty; the confirm auto-disarms after 90 s to prevent stale standing
	// one-click permanent-delete.
	mAnubis.AddSubMenuItem("──────────────────────", "").Disable()
	mTrashPreview := mAnubis.AddSubMenuItem("Empty Trash…", "Check what's in Trash and get a permanent-delete confirm button")
	mEmptyTrashConfirm := mAnubis.AddSubMenuItem("  ⚠ Delete permanently", "Permanently delete all items in Trash — cannot be undone")
	mEmptyTrashConfirm.Hide()
	wire(mScan, func() { runService(mScan, "Find stuff to clear", sirsiBin, "scan", nStore, rrAnubis) })
	wire(mJudge, func() { runCleanPreview(mJudge, mCleanConfirm, "Clear stuff…", sirsiBin, nStore, rrAnubis) })
	wire(mReview, func() { reviewCleanList() })
	wire(mCleanConfirm, func() { runCleanApply(mCleanConfirm, sirsiBin, nStore, rrAnubis) })
	wire(mKa, func() { runService(mKa, "Find leftover app files", sirsiBin, "ghosts", nStore, rrAnubis) })
	wire(mGuard, func() { spawnTUIWithCommand("guard") })
	wire(mTrashPreview, func() { runEmptyTrashPreview(mTrashPreview, mEmptyTrashConfirm, nStore, rrAnubis) })
	wire(mEmptyTrashConfirm, func() { runEmptyTrashApply(mEmptyTrashConfirm, nStore, rrAnubis) })

	systray.AddSeparator()

	// ── Recent Activity — every result, clickable to read the full detail ────
	mRecent := systray.AddMenuItem("Recent Activity", "Last 5 results — click any to read the full discovery")
	recentItems := make([]*systray.MenuItem, 5)
	for i := range recentItems {
		recentItems[i] = mRecent.AddSubMenuItem("  —", "")
		recentItems[i].Disable()
		idx := i
		wire(recentItems[idx], func() { openRecentDetail(idx) })
	}

	systray.AddSeparator()

	// ── About — surfaces & integrations (the TUI surface view) ───────────────
	addAboutSection(sirsiBin)

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Sirsi", "Exit menubar app")
	wire(mQuit, quit)

	// ── Background refresh: stats, Ra scopes, recent activity, ops, FDA ──────
	go func() {
		for {
			snap := CollectStats(cfg)
			lines := snap.FormatMenuItems()
			mStats.SetTitle(lines[0])
			mStats.SetTooltip(snap.StatusLine())

			liveState.mu.Lock()
			liveState.ramPressure = snap.RAMPressure
			liveState.mu.Unlock()
			liveState.updateTitle()

			// Ra scope rows.
			for i, item := range raScopes {
				if i < len(snap.RaScopes) {
					s := snap.RaScopes[i]
					item.SetTitle(fmt.Sprintf("  %s %s — %s", s.Icon, s.Name, s.State))
				} else {
					item.SetTitle("  —")
				}
			}

			// Recent Activity rows + the click-detail snapshot (drill-in).
			if nStore != nil {
				recent, _ := nStore.Recent(5)
				setRecentSnap(recent)
				for i, item := range recentItems {
					if i < len(recent) {
						r := recent[i]
						icon := notify.SeverityIcon(r.Severity)
						item.SetTitle(fmt.Sprintf("  %s %s — %s", icon, r.Source, r.Summary))
						item.SetTooltip("Click to read the full detail")
						item.Enable()
					} else {
						item.SetTitle("  —")
						item.Disable()
					}
				}
			}

			// Ledger board: global view, best-effort (no root = silent skip).
			if repoRoot, rerr := router.FindRepoRoot(); rerr == nil {
				if snap, serr := ledger.Build(repoRoot, "", time.Now().UTC(), ledger.DefaultStaleAfter); serr == nil {
					bs := ledger.Summarize(snap)
					pct := fmt.Sprintf("  Tasks  [%s]  %d%%", ledgerProgressBar(bs.PctDone, 12), bs.PctDone)
					mLedgerProgress.SetTitle(pct)
					mLedgerProgress.Enable()
					rows := []string{
						fmt.Sprintf("  ✅ Done: %d   🔄 Active: %d   🚫 Blocked: %d   📬 Open: %d",
							bs.DoneTasks, bs.ActiveTasks, bs.BlockedTasks, bs.OpenItems),
					}
					for _, bl := range bs.Blockers {
						rows = append(rows, fmt.Sprintf("  ⚠ %s ← %s (%s)", truncate(bl.Title, 36), bl.Agent, bl.Age))
					}
					title := "📋 Ledger"
					if bs.TotalTasks > 0 {
						title = fmt.Sprintf("📋 Ledger — %d%% done", bs.PctDone)
					}
					mLedger.SetTitle(title)
					for i, item := range ledgerRows {
						if i < len(rows) {
							item.SetTitle(rows[i])
							item.Enable()
						} else {
							item.SetTitle("  —")
							item.Disable()
						}
					}
				}
			}

			// Disk-access tier (all/some/none) — drops to hidden at full access.
			applyFDAState(mFDA)

			// Horus ops read-model rows (ADR-026 4b). Best-effort: an unresolved
			// root / collect error leaves the ops rows in place, but the Command
			// Deck still refreshes compute/risk from the local vitals snapshot.
			sum := dashboard.OpsSummary{}
			if ns, err := menubarNodeStatus(); err == nil && ns != nil {
				sum = dashboard.Summarize(ns, opsRowCount)
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
			for i, row := range commandDeckRows(snap, sum) {
				if i < len(deckRows) {
					deckRows[i].SetTitle(row)
				}
			}

			time.Sleep(cfg.Interval)
		}
	}()
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
	// Portable close-prompt: `read _` reads one line into a throwaway var and works
	// in BOTH bash and zsh. The previous `read -n 1 -s -r '?…'` is bash-only syntax
	// that errors in zsh (`zsh: not an identifier: -s`) — the user's default shell.
	commandLine += "; echo; printf 'Press Enter to close… '; read _"

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

// updateTitle paints the menubar dot with a calm, meaningful color model
// (owner 2026-06-26). The dot only "yells" (yellow) when something genuinely
// needs the human. Precedence, worst first:
//
//	🔴 red    active + bad:  failing now (rogue process, high RAM pressure)
//	🟡 yellow needs YOU:     an action only you can take (grant disk access)
//	🟢 green  active + good: running and healthy, nothing needs you
//	⚪️ white  active + idle: not yet reporting (startup / unknown)
//
// Reclaimable cache ("waste") is deliberately NOT a dot state: there is always
// some and Pantheon cleans it itself, so it must never paint the dot yellow —
// that was the old false alarm that kept the menubar permanently yellow.
func (s *menubarState) updateTitle() {
	s.mu.RLock()
	guardAlert, guardAt, ramPressure := s.guardAlert, s.guardAlertAt, s.ramPressure
	s.mu.RUnlock()

	// 🔴 RED — active + bad: something is failing right now.
	if guardAlert != "" && time.Since(guardAt) < 5*time.Minute {
		systray.SetTitle("☥ Alert")
		systray.SetTooltip(fmt.Sprintf("%s is using too much — Sirsi is calming it down", guardAlert))
		return
	}
	if ramPressure == "high" {
		systray.SetTitle("☥ Memory")
		systray.SetTooltip("Your Mac is running low on memory")
		return
	}

	// 🟡 YELLOW — needs YOU: only when Sirsi is FULLY blind (no disk access).
	// Partial access still works, so it stays calm — the "see everything" prompt
	// remains a quiet menu item rather than a standing alarm. Yellow must mean a
	// real, required action, not "you could grant more."
	switch platform.CheckDiskAccess().Level {
	case platform.AccessFull, platform.AccessSome:
		// healthy enough — fall through to green/white
	default:
		systray.SetTitle("☥ Access")
		systray.SetTooltip("Let Sirsi see your Mac so it can keep it healthy")
		return
	}

	// 🟢 GREEN / ⚪️ WHITE — nothing needs you. White until the first refresh
	// populates a snapshot (idle/unknown), green once confirmed healthy.
	if ramPressure == "" {
		systray.SetTitle("☥ Sirsi")
		systray.SetTooltip("Sirsi — starting up")
		return
	}
	systray.SetTitle("☥ Ready")
	systray.SetTooltip("Sirsi Pantheon — local intelligence control plane")
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

// rescanWaste runs one jackal scan and updates the menubar title to reflect the
// CURRENT waste. Callable on demand — critically after a clean, so the waste
// notice clears live instead of persisting until the 4h tick (the "permanent
// waste notice" bug).
func rescanWaste(ctx context.Context) {
	engine := jackal.DefaultEngine()
	engine.RegisterAll(rules.AllRules()...)
	start := time.Now()
	res, err := engine.Scan(ctx, jackal.ScanOptions{})
	if err != nil {
		return
	}
	jackal.EnrichAdvisory(res)
	_ = jackal.Persist(res, time.Since(start))
	liveState.mu.Lock()
	// Show RECLAIMABLE waste, not the full inventory: warning-tier findings (AI
	// model weights, data) are protected from one-click clean, so counting them
	// would put a scary 67 GB of Gemma weights in the title that the cleaner will
	// never remove (the false "76 GB waste" alarm). ReclaimableSize = safe+caution.
	liveState.wasteBytes = res.ReclaimableSize
	liveState.wasteLabel = jackal.FormatSize(res.ReclaimableSize) + " waste"
	liveState.mu.Unlock()
	liveState.updateTitle()
}

// startPeriodicScan scans on launch, then every 4 hours. (Clean triggers an
// immediate rescanWaste so the title never lags reality.) The 4h cadence is the
// full-rescan fallback; startLiveRefresh makes the label react to persist events
// in ~seconds, so the tray no longer "hasn't updated in 4h".
func startPeriodicScan(ctx context.Context) {
	go func() {
		for {
			rescanWaste(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(4 * time.Hour):
			}
		}
	}()
}

// liveRefreshDebounce coalesces a burst of persist writes into one label
// refresh. Long enough to absorb the multi-write bursts a `sirsi clean` makes,
// short enough to feel live — and deliberately NOT per-write (per-write refresh
// would re-amplify the very mds_stores write-storm Rail B addresses).
const liveRefreshDebounce = 1500 * time.Millisecond

// refreshFromLatest updates the tray label from the PERSISTED scan only — a
// cheap file read, no rescan, no re-persist (so it can never loop with the
// fsnotify watch that triggers it).
func refreshFromLatest() {
	ps, err := jackal.LoadLatest()
	if err != nil || ps == nil {
		return
	}
	liveState.mu.Lock()
	liveState.wasteBytes = ps.TotalSize
	liveState.wasteLabel = jackal.FormatSize(ps.TotalSize) + " waste"
	liveState.mu.Unlock()
	liveState.updateTitle()
}

// startLiveRefresh makes the menu bar reflect external state changes (e.g. a
// `sirsi clean` from the CLI re-persisting findings) within ~seconds instead of
// up to 4 hours — the user's "4 hours is lunacy" complaint. Event-driven, not a
// tighter timer: it watches the findings dir with fsnotify and refreshes the
// LABEL (from the persisted file) on a debounced write to latest-scan.json. A
// SIGUSR1 is an explicit manual trigger (e.g. a post-clean nudge). The label
// refresh is wholly separate from the resident-surface liveness heartbeat
// (A27, ≥60s) — do not conflate them.
func startLiveRefresh(ctx context.Context) {
	dir := jackal.FindingsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return
	}
	target := filepath.Clean(jackal.LatestScanPath())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGUSR1)

	go func() {
		defer watcher.Close()
		defer signal.Stop(sig)
		fire := make(chan struct{}, 1)
		var timer *time.Timer
		schedule := func() {
			if timer == nil {
				timer = time.AfterFunc(liveRefreshDebounce, func() {
					select {
					case fire <- struct{}{}:
					default:
					}
				})
				return
			}
			timer.Reset(liveRefreshDebounce)
		}
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-watcher.Events:
				if !ok {
					return
				}
				if filepath.Clean(ev.Name) == target && ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					schedule()
				}
			case <-sig:
				schedule() // manual nudge, still debounced/serialized
			case <-fire:
				refreshFromLatest()
			case _, ok := <-watcher.Errors:
				if !ok {
					return
				}
			}
		}
	}()
}

// AnkhIcon is the menu bar icon data, generated by the Ankh renderer.
var AnkhIcon = getIcon()
