// Package main — sirsi-menubar
//
// ☥ Sirsi Menu Bar Application (ADR-010)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"fyne.io/systray"
	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/notify"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
)

var version = "v0.10.0"

func main() {
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

func onReady() {
	systray.SetTemplateIcon(AnkhIcon, AnkhIcon)
	systray.SetTitle("Sirsi")
	systray.SetTooltip("Sirsi Ecosystem Monitor")

	// ── Dashboard ──────────────────────────────────────────────────
	mDashboard := systray.AddMenuItem("📊 Open Dashboard", "Open Pantheon dashboard in browser")

	// ── Stats section ───────────────────────────────────────────────
	mStats := systray.AddMenuItem("Loading...", "Click to refresh stats")
	systray.AddSeparator()

	// ── Ra section ──────────────────────────────────────────────────
	mRaHeader := systray.AddMenuItem("𓇶 Ra — Orchestrator", "Click to open Command Center")
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

	// ── Sirsi commands ─────────────────────────────────────────────
	mScan := systray.AddMenuItem("𓁢 Scan (Weigh)", "Scan for waste")
	mJudge := systray.AddMenuItem("⚖️ Judge", "Apply policies")
	mKa := systray.AddMenuItem("𓂓 Ka (Ghost Hunt)", "Detect dead apps")
	mMaat := systray.AddMenuItem("🪶 Ma'at (QA)", "Quality governance")
	mGuard := systray.AddMenuItem("🛡 Start Watchdog", "Guard --watch")

	systray.AddSeparator()

	// ── Quick access ────────────────────────────────────────────────
	mBuildLog := systray.AddMenuItem("📋 Build Log", "Open build-log.html")
	mCaseStudies := systray.AddMenuItem("📊 Case Studies", "Open case-studies.html")

	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Sirsi", "Exit menubar app")

	// ── Open notification store ─────────────────────────────────────
	nStore, _ := notify.Open(notify.DefaultPath())

	// ── Start dashboard server ──────────────────────────────────────
	cfg := DefaultStatsConfig()
	eventBuf := dashboard.NewEventBuffer(256)
	dashSrv := dashboard.New(dashboard.Config{
		Port:     dashboard.DashboardPort,
		NotifyDB: nStore,
		Events:   eventBuf,
		StatsFn: func() ([]byte, error) {
			snap := CollectStats(cfg)
			return json.Marshal(snap)
		},
	})
	if err := dashSrv.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "dashboard: %v\n", err)
	}

	// ── Background stats + recent activity loop ─────────────────────
	go func() {
		for {
			snap := CollectStats(cfg)
			lines := snap.FormatMenuItems()
			mStats.SetTitle(lines[0])
			mStats.SetTooltip(snap.StatusLine())

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

			time.Sleep(cfg.Interval)
		}
	}()

	// ── Event loop ──────────────────────────────────────────────────
	handlers := SirsiHandlers()
	raHandlers := RaHandlers()

	for {
		select {
		case <-mDashboard.ClickedCh:
			_ = dashSrv.OpenPage("/")
		case <-mStats.ClickedCh:
			snap := CollectStats(cfg)
			lines := snap.FormatMenuItems()
			mStats.SetTitle(lines[0])
			mStats.SetTooltip(snap.StatusLine())
			for i, item := range raScopes {
				if i < len(snap.RaScopes) {
					s := snap.RaScopes[i]
					item.SetTitle(fmt.Sprintf("  %s %s — %s", s.Icon, s.Name, s.State))
				}
			}
		case <-mRaHeader.ClickedCh:
			_ = OpenCommandCenter()
		case <-mRaDeploy.ClickedCh:
			raHandlers[0].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mRaKill.ClickedCh:
			raHandlers[1].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mRaCollect.ClickedCh:
			raHandlers[2].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-raScopes[0].ClickedCh:
			snap := CollectStats(cfg)
			if len(snap.RaScopes) > 0 {
				_ = OpenScopeLog(snap.RaScopes[0].Name)
			}
		case <-raScopes[1].ClickedCh:
			snap := CollectStats(cfg)
			if len(snap.RaScopes) > 1 {
				_ = OpenScopeLog(snap.RaScopes[1].Name)
			}
		case <-raScopes[2].ClickedCh:
			snap := CollectStats(cfg)
			if len(snap.RaScopes) > 2 {
				_ = OpenScopeLog(snap.RaScopes[2].Name)
			}
		case <-raScopes[3].ClickedCh:
			snap := CollectStats(cfg)
			if len(snap.RaScopes) > 3 {
				_ = OpenScopeLog(snap.RaScopes[3].Name)
			}
		case <-mScan.ClickedCh:
			handlers[0].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mJudge.ClickedCh:
			handlers[1].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mKa.ClickedCh:
			handlers[3].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mMaat.ClickedCh:
			handlers[5].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mGuard.ClickedCh:
			QuickActions()[0].ExecuteWithNotifyAndEvents(nStore, eventBuf)
		case <-mBuildLog.ClickedCh:
			_ = OpenBuildLog()
		case <-mCaseStudies.ClickedCh:
			_ = OpenCaseStudies()
		case <-mQuit.ClickedCh:
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

// AnkhIcon is the menu bar icon data, generated by the Ankh renderer.
var AnkhIcon = getIcon()
