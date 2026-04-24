# Session Handoff: UX Overhaul — Menubar + TUI Integration

**Date:** 2026-04-24
**Version:** v0.17.1
**Previous session:** 33 commits, 8,380 lines — dashboard, advisory intelligence, deity hierarchy
**Next session goal:** Make the product WORK as described below. No new features. Fix what exists.

---

## The UX Contract (Non-Negotiable)

1. **Menubar runs forever.** Ankh icon in the status bar. Always monitoring. Collects stats, runs guard watchdog, periodic lightweight scan. Never stops unless the user quits.

2. **Menubar click opens TUI.** Not a browser. A real terminal window with the BubbleTea TUI. The user types commands, sees results, interacts. When they close the window, the menubar keeps running.

3. **Visible reporting.** The menubar tooltip/title shows current state: "🟢 Clean" or "🟡 3.2 GB waste found" or "🔴 High RAM pressure." The user SEES something without clicking. When they click, the TUI shows the full picture.

4. **TUI is the interactive surface.** Not the browser dashboard. The TUI should show: scan findings, ghost count, guard status, recent notifications. The user can type `scan`, `clean`, `guard`, `ghosts`, `doctor` inside the TUI.

5. **The browser dashboard (Horus) is optional.** Power users can run `sirsi dashboard` to open it. But the primary UX is menubar → TUI.

---

## What's Broken (Current State)

### Menubar (`cmd/sirsi-menubar/main.go`)
- ❌ Clicking "Open Dashboard" opens **localhost:9119 in the browser**. Should open TUI in Terminal.app.
- ❌ **No guard/watchdog running.** The menubar imports guard but never starts `StartBridge` or `StartWatch`.
- ❌ **No periodic scan.** Menubar never runs a scan. Findings are only visible if the user manually runs `sirsi scan`.
- ❌ **No visible reporting.** Menubar title is "Sirsi" with a static ankh. Should show live state.
- ❌ **Stats refresh loop exists** but only updates menu item text (invisible unless you click). Should update the icon tooltip and title.

### TUI (`internal/output/tui.go`)
- ✅ Has notification panel (5 recent items in left pane)
- ✅ Has command execution (type command → runs sirsi binary → shows output)
- ✅ Has deity roster with active detection
- ❌ **No scan findings view.** TUI shows command output but doesn't read persisted findings.
- ❌ **No guard integration.** TUI doesn't start or show watchdog status.
- ❌ **Cannot be opened from menubar.** No mechanism for menubar to spawn a TUI window.

### Dashboard (`internal/dashboard/`)
- ✅ 29 API endpoints, all working
- ✅ Terminal-first SPA with command input
- ❌ **Browser-based.** Should be TUI-based for the primary UX.
- The APIs are still valuable — the TUI and MCP can call them.

---

## What Needs to Be Built

### 1. Menubar → TUI Bridge
**File:** `cmd/sirsi-menubar/main.go`

When user clicks the menubar ankh or "Open Horus" menu item:
```go
// Instead of:
_ = dashSrv.OpenPage("/")

// Do:
cmd := exec.Command("osascript", "-e",
    `tell application "Terminal" to do script "sirsi"`)
cmd.Start()
```
This opens Terminal.app with `sirsi` (which launches the TUI gateway). On iTerm2, use the iTerm2 AppleScript equivalent. The `ra.SpawnWindow()` function already knows how to do this.

### 2. Menubar Runs Guard
**File:** `cmd/sirsi-menubar/main.go`

Start the guard watchdog in the menubar background loop:
```go
bridge := guard.StartBridge(ctx, guard.BridgeConfig{
    WatchConfig: guard.WatchConfig{AutoRenice: true},
    OnAlert: func(alert guard.AlertEntry) {
        // Update menubar icon/title to show alert
        systray.SetTitle("⚠️ " + alert.ProcessName)
    },
})
```

### 3. Menubar Periodic Scan
**File:** `cmd/sirsi-menubar/main.go`

Run a lightweight scan every 4 hours (or on first launch):
```go
go func() {
    for {
        engine := jackal.DefaultEngine()
        engine.RegisterAll(rules.AllRules()...)
        res, _ := engine.Scan(ctx, jackal.ScanOptions{})
        jackal.EnrichAdvisory(res)
        jackal.Persist(res, 0)
        
        // Update menubar title
        if res.TotalSize > 1<<30 { // > 1 GB
            systray.SetTitle(fmt.Sprintf("🟡 %s waste", jackal.FormatSize(res.TotalSize)))
        } else {
            systray.SetTitle("🟢 Clean")
        }
        
        time.Sleep(4 * time.Hour)
    }
}()
```

### 4. TUI Reads Persisted Findings
**File:** `internal/output/tui.go`

Add a `viewFindings()` function that loads `~/.config/pantheon/findings/latest-scan.json` and renders the category breakdown with advisories. Similar to how the dashboard's `viewScan` works but in BubbleTea lipgloss rendering.

### 5. Menubar Title Shows State
**File:** `cmd/sirsi-menubar/main.go`

The menubar title should cycle or show:
- "🟢 Sirsi" — all clean, low RAM
- "🟡 Sirsi 12 GB" — waste found
- "🔴 Sirsi RAM" — high RAM pressure
- "⚠️ Sirsi node" — process alert active

### 6. "Renice" → Plain Language
Everywhere "renice" appears in user-facing text, replace with:
"Deprioritize background processes (safe, reversible)"

---

## Files to Modify

| File | Change |
|------|--------|
| `cmd/sirsi-menubar/main.go` | Bridge to TUI, start guard, periodic scan, live title |
| `internal/output/tui.go` | Add findings view, guard status, scan command |
| `cmd/sirsi/dashboard.go` | Keep as optional browser dashboard for power users |
| `internal/dashboard/server.go` | No changes — APIs still serve TUI and MCP |
| `internal/guard/bridge.go` | No changes — already supports OnAlert callback |
| `internal/jackal/persist.go` | No changes — already persists findings |

---

## What NOT to Change

- The dashboard APIs (29 endpoints) stay. They serve the TUI, MCP, and browser.
- The scan engine stays. 81 rules, advisory intelligence, all working.
- The deity hierarchy stays. ADR-015 is accepted.
- The VS Code extension stays. Diagnostic provider works.

---

## Testing the UX

After implementation, verify this exact flow:

1. `sirsi-menubar` starts → ankh appears in menu bar
2. Wait 30 seconds → menubar title shows waste count or "Clean"
3. Click ankh → Terminal.app opens with `sirsi` TUI
4. Type `scan` in TUI → findings appear with advisories
5. Type `clean` → safe items cleaned, findings count drops
6. Close Terminal window → menubar still running, ankh still there
7. Wait for guard alert → menubar title changes to show alert
8. Click ankh again → TUI opens, shows the alert in notifications

If all 8 steps work, the product is ready to ship.

---

## Uncommitted Files

None — all 33 commits from this session are committed.

## Architecture Decisions This Session

- ADR-015: Deity Hierarchy (Horus=local, Ra=fleet)
- Dashboard renamed to Horus
- WorkstationReport struct for Horus→Ra transport
- Advisory intelligence on all findings (advisory.go)
- Severity classification (safe/caution/warning)

## Key Metrics

- 81 scan rules (was 58)
- 628 findings found, all fixable, 624 cleaned, ~30 GB reclaimed
- 29 dashboard API endpoints
- 14 MB stripped binary
- 0 test failures across 34 packages
