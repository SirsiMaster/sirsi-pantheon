// Package main — sirsi-menubar
//
// ☥ Sirsi Menu Bar Application (ADR-010)
package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/apprecovery"
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

type controlPlane struct {
	ctx                 context.Context
	cancel              context.CancelFunc
	dashboard           *dashboard.Server
	notifications       *notify.Store
	sirsiBin            string
	statsConfig         StatsConfig
	sneLocalAccessToken string
	routerRoot          string
	routerThreadID      string
	stopOnce            sync.Once
}

var residentControlPlane *controlPlane

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

	cp, err := initializeResidentControlPlane(startControlPlane)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Pantheon control plane: %v\n", err)
		os.Exit(1)
	}
	defer cp.stop()

	if os.Getenv("SIRSI_HEADLESS") == "1" {
		runHeadless(cp)
		return
	}

	go stopControlPlaneOnSignal(cp, systray.Quit)
	systray.Run(onReady, onExit)
}

func initializeResidentControlPlane(start func() (*controlPlane, error)) (*controlPlane, error) {
	cp, err := start()
	if err != nil {
		return nil, err
	}
	residentControlPlane = cp
	return cp, nil
}

func runHeadless(cp *controlPlane) {
	fmt.Printf("☥ Sirsi Menubar %s (Headless Mode)\n", version)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh
	cp.stop()
}

func stopControlPlaneOnSignal(cp *controlPlane, quitUI func()) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)
	<-sigCh
	cp.stop()
	quitUI()
}

func startControlPlane() (*controlPlane, error) {
	traceStartupMemory("start")
	sirsiBin := findSirsiBinary()
	nStore, _ := notify.Open(notify.DefaultPath())
	traceStartupMemory("notify-open")
	cfg := DefaultStatsConfig()
	eventBuf := dashboard.NewEventBuffer(256)
	traceStartupMemory("event-buffer")
	appRecovery, recoveryErr := apprecovery.LoadDefaultManager()
	traceStartupMemory("app-recovery")
	if recoveryErr != nil {
		fmt.Fprintf(os.Stderr, "application recovery: %v\n", recoveryErr)
	}
	sneLocalAccessToken, accessErr := dashboard.LoadOrCreateDefaultSNELocalAccessToken()
	traceStartupMemory("access-token")
	sneLocalAccessTokenPath, pathErr := dashboard.DefaultSNELocalAccessTokenPath()
	if pathErr != nil && accessErr == nil {
		accessErr = pathErr
	}
	if accessErr != nil {
		fallback := make([]byte, 32)
		if _, err := rand.Read(fallback); err != nil {
			if nStore != nil {
				nStore.Close()
			}
			return nil, fmt.Errorf("SNE local capability unavailable: %w", accessErr)
		}
		sneLocalAccessToken = base64.RawURLEncoding.EncodeToString(fallback)
		fmt.Fprintf(os.Stderr, "SNE local capability unavailable; protected operations fail closed: %v\n", accessErr)
	}

	dashSrv := dashboard.New(dashboard.Config{
		Port:                    dashboard.DashboardPort,
		NotifyDB:                nStore,
		Events:                  eventBuf,
		SirsiBin:                sirsiBin,
		SNELocalAccessToken:     sneLocalAccessToken,
		SNELocalAccessTokenPath: sneLocalAccessTokenPath,
		StatsFn: func() ([]byte, error) {
			snap := CollectStats(cfg)
			return json.Marshal(snap)
		},
		NodeStatusFn: menubarNodeStatus,
		LedgerFn:     menubarLedger,
		FabricFn:     menubarFabric,
		AppRecovery:  appRecovery,
		SNEInstall:   dashboard.DefaultSNEInstallConfig(),
		SNELifecycle: dashboard.DefaultSNELifecycleConfig(),
	})
	traceStartupMemory("dashboard-new")
	if err := dashSrv.Start(); err != nil {
		if nStore != nil {
			nStore.Close()
		}
		return nil, fmt.Errorf("start dashboard: %w", err)
	}
	traceStartupMemory("dashboard-start")

	ctx, cancel := context.WithCancel(context.Background())
	cp := &controlPlane{
		ctx:                 ctx,
		cancel:              cancel,
		dashboard:           dashSrv,
		notifications:       nStore,
		sirsiBin:            sirsiBin,
		statsConfig:         cfg,
		sneLocalAccessToken: sneLocalAccessToken,
	}
	startGuardBridge(ctx)
	traceStartupMemory("guard-bridge")
	startPeriodicScan(ctx)
	traceStartupMemory("periodic-scan")
	startLiveRefresh(ctx)
	traceStartupMemory("live-refresh")
	cp.routerRoot, cp.routerThreadID = registerMenubarThread(ctx)
	traceStartupMemory("router-register")
	return cp, nil
}

func traceStartupMemory(stage string) {
	if os.Getenv("SIRSI_STARTUP_MEMORY_TRACE") != "1" {
		return
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(os.Stderr, "pantheon_memory stage=%s heap_alloc=%d heap_sys=%d heap_idle=%d heap_released=%d\n", stage, m.HeapAlloc, m.HeapSys, m.HeapIdle, m.HeapReleased)
}

func (cp *controlPlane) stop() {
	if cp == nil {
		return
	}
	cp.stopOnce.Do(func() {
		if cp.cancel != nil {
			cp.cancel()
		}
		closeMenubarThread(cp.routerRoot, cp.routerThreadID)
		if cp.dashboard != nil {
			_ = cp.dashboard.Stop()
		}
		if cp.notifications != nil {
			cp.notifications.Close()
		}
	})
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

// menubarLedger produces the compact ledger.BoardSummary for GET /api/ledger
// (A26 Nexus seam). Mirrors the root-resolution pattern from menubarNodeStatus.
func menubarLedger() (ledger.BoardSummary, error) {
	routerRoot, ok := resolveRouterRoot()
	if !ok {
		return ledger.BoardSummary{}, fmt.Errorf("router root not resolvable from menubar context")
	}
	repoRoot := filepath.Dir(filepath.Dir(routerRoot)) // strip /.agents/idea-router
	snap, err := ledger.Build(repoRoot, "", time.Now().UTC(), 0)
	if err != nil {
		return ledger.BoardSummary{}, fmt.Errorf("ledger build: %w", err)
	}
	return ledger.Summarize(snap), nil
}

func menubarFabric() (ledger.FabricBoard, error) {
	routerRoot, ok := resolveRouterRoot()
	if !ok {
		return ledger.FabricBoard{}, fmt.Errorf("router root not resolvable from menubar context")
	}
	repoRoot := filepath.Dir(filepath.Dir(routerRoot))
	board, err := ledger.BuildFabric(repoRoot, version, time.Now().UTC())
	if err != nil {
		return ledger.FabricBoard{}, fmt.Errorf("fabric build: %w", err)
	}
	return board, nil
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

	cp := residentControlPlane
	if cp == nil {
		fmt.Fprintln(os.Stderr, "Pantheon control plane was not initialized")
		systray.Quit()
		return
	}
	sirsiBin := cp.sirsiBin
	nStore := cp.notifications
	cfg := cp.statsConfig
	sneLocalAccessToken := cp.sneLocalAccessToken

	// First launch after a DMG/GUI install drives the same surface + permission
	// wizard as the CLI, so "the GUI install implements this" is literal.
	maybeFirstRunSetup()

	quit := func() {
		cp.stop()
		systray.Quit()
	}

	// ── Command Deck — local intelligence control plane ─────────────────────
	mDeck := systray.AddMenuItem("☥ Command Deck", "Live local AI, compute, router, context, and risk")
	rrDeck := newDeityResult(mDeck)
	const deckRowCount = 5
	deckRows := make([]*systray.MenuItem, deckRowCount)
	for i := range deckRows {
		deckRows[i] = mDeck.AddSubMenuItem("  —", "Live Sirsi control-plane signal")
		deckRows[i].Disable()
	}
	mAskSirsi := mDeck.AddSubMenuItem("Open Nexus Local AI…", "Open the governed on-device SNE conversation surface")
	mGemmaServe := mDeck.AddSubMenuItem("Start / Check Gemma Broker…", "Run the warm local Gemma broker")
	mRouterDoctor := mDeck.AddSubMenuItem("Router Doctor…", "Inspect and repair safe router liveness issues")
	mComputeProfile := mDeck.AddSubMenuItem("Compute Profile…", "Inspect Apple Silicon, GPU, and Neural Engine lanes")
	mSafeRestart := mDeck.AddSubMenuItem("Restart & Resume Safely…", "Planned FileVault restart; Apple requests your password and Pantheon resumes after login")
	wire(mAskSirsi, func() {
		launchURL, err := buildMenubarNexusURL(sneLocalAccessToken)
		if err != nil {
			fmt.Fprintf(os.Stderr, "build Nexus local AI launch: %v\n", err)
			return
		}
		if err := exec.Command("open", launchURL).Start(); err != nil {
			fmt.Fprintf(os.Stderr, "open Nexus local AI: %v\n", err)
		}
	})
	wire(mGemmaServe, func() { runCanonicalAction(mGemmaServe, "gemma/serve", nStore, rrDeck) })
	wire(mRouterDoctor, func() { runCanonicalAction(mRouterDoctor, "router/doctor", nStore, rrDeck) })
	wire(mComputeProfile, func() { runCanonicalAction(mComputeProfile, "compute", nStore, rrDeck) })
	wire(mSafeRestart, func() { spawnTUIWithCommand(authenticatedRestartCommand) })

	// ── Status header (top level) ───────────────────────────────────────────
	mStats := systray.AddMenuItem("Vitals loading…", "Workstation status — click to run full diagnostics in place")
	wire(mStats, func() { runCanonicalAction(mStats, "doctor", nStore, nil) })
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
	rrHorus := newDeityResult(mHorus)
	mOpsHeader := mHorus.AddSubMenuItem("ops: loading…", "Horus local-node status")
	const opsRowCount = 12
	opsRows := make([]*systray.MenuItem, opsRowCount)
	for i := range opsRows {
		opsRows[i] = mHorus.AddSubMenuItem("  —", "")
	}
	mDashboard := mHorus.AddSubMenuItem("↳ Open Full Dashboard…", "Open the native Pantheon web control surface")
	wire(mOpsHeader, func() { runCanonicalAction(mOpsHeader, "status", nStore, rrHorus) })
	wire(mDashboard, func() { _ = exec.Command("open", "http://127.0.0.1:9119/horus").Start() })
	for _, row := range opsRows {
		item := row
		wire(item, func() { _ = exec.Command("open", "http://127.0.0.1:9119/horus").Start() })
	}

	// ── 𓆄 Ma'at — Quality ────────────────────────────────────────────────────
	mMaatTop := systray.AddMenuItem("𓆄 Ma'at — Quality", "Quality + governance audit, system diagnostics")
	rrMaat := newDeityResult(mMaatTop)
	mMaat := mMaatTop.AddSubMenuItem("Quality Audit", "Run the workstation quality + governance audit")
	mDiag := mMaatTop.AddSubMenuItem("System Diagnostics", "Run sirsi diagnose — full health checks")
	wire(mMaat, func() { runCanonicalAction(mMaat, "quality", nStore, rrMaat) })
	wire(mDiag, func() { runCanonicalAction(mDiag, "doctor", nStore, rrMaat) })

	// ── 𓁢 Thoth — Memory ─────────────────────────────────────────────────────
	mThothTop := systray.AddMenuItem("𓁢 Thoth — Memory", "Project memory + knowledge ingestion")
	rrThoth := newDeityResult(mThothTop)
	mThoth := mThothTop.AddSubMenuItem("Sync Memory", "Sync project memory from source + git history")
	mSeshat := mThothTop.AddSubMenuItem("Ingest Knowledge", "Ingest configured knowledge sources")
	wire(mThoth, func() { runCanonicalAction(mThoth, "thoth/sync", nStore, rrThoth) })
	wire(mSeshat, func() { runCanonicalAction(mSeshat, "seshat/ingest", nStore, rrThoth) })

	// ── 𓇶 Ra — Agent Fleet ───────────────────────────────────────────────────
	// Deploy/Kill spawn/terminate windows → Terminal path (Rule A1: no silent
	// one-click fleet mutation). Collect is read-only → in-place.
	mRa := systray.AddMenuItem("𓇶 Ra — Agent Fleet", "AI agent orchestration")
	rrRa := newDeityResult(mRa)
	mRaDeploy := mRa.AddSubMenuItem("Deploy All Scopes", "sirsi ra deploy")
	mRaDeployConfirm := mRa.AddSubMenuItem("  ⚠ Confirm deploy", "Commit the exact prepared deployment")
	mRaDeployConfirm.Hide()
	mRaKill := mRa.AddSubMenuItem("Kill All Windows", "sirsi ra kill")
	mRaKillConfirm := mRa.AddSubMenuItem("  ⚠ Confirm kill", "Commit the exact prepared fleet termination")
	mRaKillConfirm.Hide()
	mRaCollect := mRa.AddSubMenuItem("Collect Results", "sirsi ra collect")
	raScopes := make([]*systray.MenuItem, 4)
	for i := range raScopes {
		raScopes[i] = mRa.AddSubMenuItem("  —", "Click to view scope status")
		raScopes[i].Disable()
	}
	var raDeployState, raKillState preparedMenuAction
	wire(mRaDeploy, func() {
		prepareCanonicalAction(mRaDeploy, mRaDeployConfirm, "ra/deploy", "all-scopes", nStore, rrRa, &raDeployState)
	})
	wire(mRaDeployConfirm, func() { commitCanonicalAction(mRaDeployConfirm, nStore, rrRa, &raDeployState) })
	wire(mRaKill, func() {
		prepareCanonicalAction(mRaKill, mRaKillConfirm, "ra/kill", "all-windows", nStore, rrRa, &raKillState)
	})
	wire(mRaKillConfirm, func() { commitCanonicalAction(mRaKillConfirm, nStore, rrRa, &raKillState) })
	wire(mRaCollect, func() { runCanonicalAction(mRaCollect, "ra/collect", nStore, rrRa) })

	// ── 𓋹 Insight — Hardware / Risk / Consistency ────────────────────────────
	mInsight := systray.AddMenuItem("𓋹 Insight", "Hardware, uncommitted risk, and consistency")
	rrInsight := newDeityResult(mInsight)
	mSeba := mInsight.AddSubMenuItem("Hardware Info", "CPU, GPU, and accelerator summary")
	mOsiris := mInsight.AddSubMenuItem("Uncommitted Risk", "Assess risk from uncommitted work")
	mNet := mInsight.AddSubMenuItem("Consistency Check", "Validate cross-module consistency")
	wire(mSeba, func() { runCanonicalAction(mSeba, "hardware", nStore, rrInsight) })
	wire(mOsiris, func() { runCanonicalAction(mOsiris, "risk", nStore, rrInsight) })
	wire(mNet, func() { runCanonicalAction(mNet, "net/align", nStore, rrInsight) })

	// ── SNE — governed local inference lifecycle ─────────────────────────────
	mSNE := systray.AddMenuItem("☥ SNE — Local AI", "Inspect and control the governed local inference service")
	rrSNE := newDeityResult(mSNE)
	mSNEStatus := mSNE.AddSubMenuItem("Check SNE Readiness", "Read the exact installed model/runtime readiness")
	mSNEStart := mSNE.AddSubMenuItem("Start / Restore SNE", "Start the accepted SNE service")
	mSNEStop := mSNE.AddSubMenuItem("Stop SNE…", "Prepare a supervised SNE stop")
	mSNEStopConfirm := mSNE.AddSubMenuItem("  ⚠ Confirm stop", "Commit the exact prepared SNE stop")
	mSNEStopConfirm.Hide()
	mSNEQuarantine := mSNE.AddSubMenuItem("Quarantine SNE…", "Hold SNE offline across automatic recovery")
	mSNEQuarantineConfirm := mSNE.AddSubMenuItem("  ⚠ Confirm quarantine", "Commit the exact prepared quarantine")
	mSNEQuarantineConfirm.Hide()
	mSNEOpen := mSNE.AddSubMenuItem("Open SNE Control Center…", "Models, readiness, diagnostics, and support bundle")
	var sneStopState, sneQuarantineState preparedMenuAction
	wire(mSNEStatus, func() { runCanonicalAction(mSNEStatus, "gemma/status", nStore, rrSNE) })
	wire(mSNEStart, func() { runCanonicalAction(mSNEStart, "gemma/serve", nStore, rrSNE) })
	wire(mSNEStop, func() {
		prepareCanonicalAction(mSNEStop, mSNEStopConfirm, "gemma/stop", "local-sne", nStore, rrSNE, &sneStopState)
	})
	wire(mSNEStopConfirm, func() { commitCanonicalAction(mSNEStopConfirm, nStore, rrSNE, &sneStopState) })
	wire(mSNEQuarantine, func() {
		prepareCanonicalAction(mSNEQuarantine, mSNEQuarantineConfirm, "gemma/quarantine", "local-sne", nStore, rrSNE, &sneQuarantineState)
	})
	wire(mSNEQuarantineConfirm, func() { commitCanonicalAction(mSNEQuarantineConfirm, nStore, rrSNE, &sneQuarantineState) })
	wire(mSNEOpen, func() { _ = exec.Command("open", "http://127.0.0.1:9119/sne").Start() })

	// ── Repairs — diagnosis must lead to resolution ─────────────────────────
	mRepairs := systray.AddMenuItem("𓁐 Repairs & Recovery", "Prepare, confirm, execute, and verify system repairs")
	rrRepairs := newDeityResult(mRepairs)
	mRepairDiagnosed := mRepairs.AddSubMenuItem("Repair Diagnosed Issues…", "Prepare Pantheon's bounded safe repairs")
	mRepairConfirm := mRepairs.AddSubMenuItem("  ⚠ Confirm repairs", "Commit the exact prepared repair")
	mRepairConfirm.Hide()
	mNetworkAudit := mRepairs.AddSubMenuItem("Audit Network", "Inspect DNS, firewall, TLS, Wi-Fi, and VPN")
	mNetworkFix := mRepairs.AddSubMenuItem("Repair Network…", "Prepare bounded network repair")
	mNetworkFixConfirm := mRepairs.AddSubMenuItem("  ⚠ Confirm network repair", "Commit the exact prepared network repair")
	mNetworkFixConfirm.Hide()
	mUpdate := mRepairs.AddSubMenuItem("Install Signed Update…", "Prepare Pantheon's signed application updater")
	mUpdateConfirm := mRepairs.AddSubMenuItem("  ⚠ Confirm signed update", "Commit the exact prepared signed update")
	mUpdateConfirm.Hide()
	mPermissions := mRepairs.AddSubMenuItem("Review Full Disk Access…", "Open macOS permission settings; Pantheon verifies after return")
	mRecoveryCenter := mRepairs.AddSubMenuItem("Open Recovery Center…", "Application restart/resume targets and receipts")
	var repairState, networkState, updateState preparedMenuAction
	wire(mRepairDiagnosed, func() {
		prepareCanonicalAction(mRepairDiagnosed, mRepairConfirm, "system/fix", "this-mac", nStore, rrRepairs, &repairState)
	})
	wire(mRepairConfirm, func() { commitCanonicalAction(mRepairConfirm, nStore, rrRepairs, &repairState) })
	wire(mNetworkAudit, func() { runCanonicalAction(mNetworkAudit, "network", nStore, rrRepairs) })
	wire(mNetworkFix, func() {
		prepareCanonicalAction(mNetworkFix, mNetworkFixConfirm, "network/fix", "this-mac", nStore, rrRepairs, &networkState)
	})
	wire(mNetworkFixConfirm, func() { commitCanonicalAction(mNetworkFixConfirm, nStore, rrRepairs, &networkState) })
	wire(mUpdate, func() {
		prepareCanonicalAction(mUpdate, mUpdateConfirm, "update/app", "Pantheon.app", nStore, rrRepairs, &updateState)
	})
	wire(mUpdateConfirm, func() { commitCanonicalAction(mUpdateConfirm, nStore, rrRepairs, &updateState) })
	wire(mPermissions, func() { openFullDiskAccessPane(nStore) })
	wire(mRecoveryCenter, func() { _ = exec.Command("open", "http://127.0.0.1:9119/").Start() })

	// ── Fabric — canonical work and message streams ────────────────────────────
	const ledgerRowCount = 5
	mLedger := systray.AddMenuItem("𓂀 Fabric — loading…", "Canonical fabric work and message status")
	mLedgerProgress := mLedger.AddSubMenuItem("  Loading last good payload…", "Completed and in-flight work")
	mLedgerProgress.Disable()
	ledgerRows := make([]*systray.MenuItem, ledgerRowCount)
	for i := range ledgerRows {
		ledgerRows[i] = mLedger.AddSubMenuItem("  —", "")
	}
	mLedgerOpen := mLedger.AddSubMenuItem("View full ledger…", "Run sirsi router ledger in Terminal")
	wire(mLedgerOpen, func() { runCanonicalAction(mLedgerOpen, "router/ledger", nStore, nil) })
	for _, row := range ledgerRows {
		item := row
		wire(item, func() { _ = exec.Command("open", "http://127.0.0.1:9119/horus").Start() })
	}

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
	mGuardCenter := mAnubis.AddSubMenuItem("Open Process Relief…", "Inspect, renice, or stop a selected process with confirmation")
	mGhostCenter := mAnubis.AddSubMenuItem("Open App-Leftover Cleanup…", "Review and clean selected ghost-app files")
	mVaultCenter := mAnubis.AddSubMenuItem("Open Vault Maintenance…", "Search and prune governed retained artifacts")
	// Permanent delete — owner request 2026-08-03. Two-click: preview arms the
	// confirm item (auto-disarms after 30s); confirm executes. Rule A1 compliant.
	mAnubis.AddSubMenuItem("──────────", "").Disable()
	mPermDelete := mAnubis.AddSubMenuItem("🗑 Permanently Delete Trash…", "Check what's in Trash, then permanently delete it")
	mPermDeleteConfirm := mAnubis.AddSubMenuItem("  ⚠ Confirm: permanently delete", "This cannot be undone — files are gone forever")
	mPermDeleteConfirm.Hide()
	wire(mScan, func() { runCanonicalAction(mScan, "scan", nStore, rrAnubis) })
	wire(mJudge, func() { runCleanPreview(mJudge, mCleanConfirm, "Clear stuff…", sirsiBin, nStore, rrAnubis) })
	wire(mReview, func() { reviewCleanList() })
	wire(mCleanConfirm, func() { runCleanApply(mCleanConfirm, sirsiBin, nStore, rrAnubis) })
	wire(mKa, func() { runCanonicalAction(mKa, "ghosts", nStore, rrAnubis) })
	wire(mGuard, func() { runCanonicalAction(mGuard, "guard", nStore, rrAnubis) })
	wire(mGuardCenter, func() { _ = exec.Command("open", "http://127.0.0.1:9119/guard").Start() })
	wire(mGhostCenter, func() { _ = exec.Command("open", "http://127.0.0.1:9119/ghosts").Start() })
	wire(mVaultCenter, func() { _ = exec.Command("open", "http://127.0.0.1:9119/vault").Start() })
	wire(mPermDelete, func() { runPermanentDelete(mPermDelete, mPermDeleteConfirm, nStore) })
	wire(mPermDeleteConfirm, func() { runPermanentDeleteApply(mPermDeleteConfirm, nStore) })

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
		var lastGoodFabric time.Time
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

			// The menubar consumes the same FabricBoard producer as /api/fabric.
			// Failures retain the prior payload and label its age; they never turn
			// an unavailable producer into authoritative zeroes.
			if board, ferr := menubarFabric(); ferr == nil {
				lastGoodFabric = time.Now().UTC()
				mLedger.SetTitle(fmt.Sprintf("𓂀 Fabric — %d completed / %d in flight", board.Work.Done, board.Work.Open))
				mLedgerProgress.SetTitle(fmt.Sprintf("  Completed / in flight: %d / %d", board.Work.Done, board.Work.Open))
				mLedgerProgress.Enable()
				rows := []string{
					fmt.Sprintf("  In progress / assigned: %d / %d", board.Work.InProgress, board.Work.AssignedNotStarted),
					fmt.Sprintf("  Stalled / blocked / idle: %d / %d / %d", board.Work.Stalled, board.Work.Blocked, countIdleLanes(board.Lanes)),
					fmt.Sprintf("  %d messages pending", board.Messages.Open),
				}
				for _, lane := range board.Lanes {
					if lane.State == "IDLE with work" || lane.State == "BLOCKED" {
						rows = append(rows, fmt.Sprintf("  ⚠ %s — %s", lane.Agent, lane.State))
					}
				}
				for i, item := range ledgerRows {
					if i < len(rows) {
						item.SetTitle(rows[i])
						item.Enable()
					} else {
						item.SetTitle("  —")
						item.Disable()
					}
				}
			} else if lastGoodFabric.IsZero() {
				mLedger.SetTitle("𓂀 Fabric — unavailable (no payload)")
			} else {
				mLedger.SetTitle(fmt.Sprintf("𓂀 Fabric — stale · %s old", formatAge(time.Since(lastGoodFabric))))
			}

			// Disk-access tier (all/some/none) — drops to hidden at full access.
			applyFDAState(mFDA)

			// Horus ops read-model rows (ADR-026 4b). Every collection attempt
			// replaces the prior render, including failures: retaining old values
			// would present stale success as current truth.
			ns, collectErr := menubarNodeStatus()
			sum, lead, rows := opsSnapshotRows(ns, collectErr, opsRowCount, time.Now().UTC())
			mOpsHeader.SetTitle(lead)
			for i, item := range opsRows {
				if i < len(rows) {
					item.SetTitle(rows[i])
				} else {
					item.SetTitle("  —")
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

func buildMenubarNexusURL(token string) (string, error) {
	return dashboard.BuildNexusCapabilityURL(dashboard.NexusLocalAIURL, token)
}

func onExit() {}

func countIdleLanes(lanes []ledger.FabricLane) int {
	count := 0
	for _, lane := range lanes {
		if lane.State == "IDLE" || lane.State == "IDLE with work" {
			count++
		}
	}
	return count
}

func formatAge(age time.Duration) string {
	if age < time.Minute {
		return "<1m"
	}
	if age < time.Hour {
		return fmt.Sprintf("%dm", int(age.Minutes()))
	}
	return fmt.Sprintf("%dh%02dm", int(age.Hours()), int(age.Minutes())%60)
}

// ── TUI Bridge ─────────────────────────────────────────────────────────

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
	res, err := engine.Scan(ctx, jackal.ScanOptions{Manifest: jackal.NewPlatformManifest()})
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

var refreshFromLatestFn = refreshFromLatest

// startPeriodicScan retains its historical name for call-site compatibility,
// but resident Pantheon no longer schedules filesystem scans. Spotlight owns
// broad macOS indexing; Pantheon hydrates its governed persisted manifest and
// receives event-driven updates when an explicit Scan/Clean operation publishes
// a successor. This prevents two background indexers from walking the same disk.
// Explicit user-requested forensic scans remain available through the CLI/UI.
func startPeriodicScan(ctx context.Context) {
	_ = ctx
	refreshFromLatestFn()
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
