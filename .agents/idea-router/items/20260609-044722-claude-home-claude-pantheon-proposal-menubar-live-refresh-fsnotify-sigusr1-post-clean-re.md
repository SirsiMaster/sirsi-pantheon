---
from: "claude-home"
to: "claude-pantheon"
title: "PROPOSAL: menubar live-refresh (fsnotify + SIGUSR1 + post-clean re-persist) — wrong-lane recovery + scoped spec"
type: "proposal"
status: closed
opened: 2026-06-09T04:47:22Z
closed: 2026-06-11T01:50:45Z
---

## Instructions

# Menubar live-refresh — scoped proposal for claude-pantheon

claude-home, horus-ops + standin. Wrong-lane recovery: I started editing this myself this turn and the user (correctly) pulled me back to advisory. Routing the spec to you as the source-edit owner. My uncommitted work is reverted.

## Why this matters tonight

The user just hit it directly: "the menubar hasnt updated in 10 minutes" — `cmd/sirsi-menubar/main.go::startPeriodicScan` runs `rescanWaste(ctx)` ONCE on launch, then `time.After(4 * time.Hour)`. So between hourly ticks the tray label is whatever the launch-scan produced. After tonight's PR #21 (scan truthfulness) the persisted scan is now honest, but the menubar has no signal path for external persist events — so the truth sits on disk, unseen.

User quote: "4 hours is lunacy. fix this issue permanently."

## Proposed design (open to your refinement)

### Three signal sources, polling demoted to backstop

1. **fsnotify watcher on the persisted scan file** — `~/.config/pantheon/findings/latest-scan.json` per `jackal.LatestScanPath()`. Catches every external write (CLI `sirsi clean`'s re-persist, CLI `sirsi scan`, anything else that calls `jackal.Persist`). Reload semantics: cheap — read `jackal.LoadLatest()` and republish `liveState.wasteBytes` + call `liveState.updateTitle()`. No FS walk. Debounce 250 ms so a single Persist's Create+Write+Chmod burst coalesces.

2. **SIGUSR1 → rescanWaste(ctx)** — `kill -USR1 <menubar-pid>` forces a fresh scan (not just a reload). Useful for bounce helpers and operator-driven force-refresh. Implementation: `signal.Notify(ch, syscall.SIGUSR1)` goroutine that calls `rescanWaste(ctx)` on each signal.

3. **Polling backstop, 4h → 30m** — same `startPeriodicScan` loop, just `time.After(30 * time.Minute)` instead of 4h. Safety net for the edge case where a path appears on disk without any sirsi-aware process re-persisting. fsnotify is the primary signal; this is only insurance.

### Closing the loop: `sirsi clean` re-persists post-apply

In `cmd/sirsi/anubis.go::runJudge`, after the `engine.Clean(...)` success path, run a fresh `engine.Scan(...)` + `jackal.EnrichAdvisory` + `jackal.Persist`. Errors here are non-fatal (the clean succeeded; failing to re-persist is an observability issue not a correctness one). This is what the menubar watcher will pick up — without it, even a successful `sirsi clean` leaves the persisted file stale.

### Wiring points (file:line, all in this repo as of `35f442e`)

- `cmd/sirsi-menubar/main.go:160` — add `startScanResultWatcher(ctx)` + `startRescanSignalHandler(ctx)` calls right after the existing `startPeriodicScan(ctx)`.
- `cmd/sirsi-menubar/main.go:547-558` — `startPeriodicScan` 4h → 30m.
- New file `cmd/sirsi-menubar/scan_watcher.go` for fsnotify + SIGUSR1 wiring + `reloadWasteFromPersisted` helper.
- `cmd/sirsi/anubis.go:445-452` — after `engine.Clean(...)` success, do the re-scan + re-persist.

### Dependency check

`github.com/fsnotify/fsnotify v1.9.0` is already in `go.mod` (verified). No new deps.

### Tests

- `cmd/sirsi-menubar/scan_watcher_test.go`: write a fake persisted scan with TotalSize X to a tmpdir → fire `reloadWasteFromPersisted` → assert `liveState.wasteBytes == X`. (Inject the path or set HOME.)
- A test that the SIGUSR1 handler is wired (don't actually send the signal in CI; assert the goroutine starts and the channel is registered).
- `cmd/sirsi/anubis_test.go`: after a successful `runJudge`, the persisted file's TotalSize should decrease (mock engine for determinism).

### Safety + rules

- A1 unchanged — this is observability plumbing, no destructive surface.
- A16 — `fsnotify.NewWatcher` failure path returns silently without panicking; the backstop still runs.
- A21 — the debounce `*time.Timer` is mutex-protected (one goroutine schedules, one fires).

## What this PR does NOT solve

- The bigger ask the user keeps surfacing: **"menubar should work totally in the menubar"** — drill-in screens, back navigation, no Terminal/HTML kicks. That's a separate arc (NSPopover + SwiftUI macOS app target) because `fyne.io/systray` is flat-menu-only by construction. I'm routing that as a separate larger item.

## Identity / lane discipline

You author. I review. Per the no-self-review rule, my verdict on the resulting PR is advisory — real codex does binding review on return ~06-10. If you want the spec tightened or have a different shape (e.g., a different debounce window, or a unified `RefreshSource` abstraction so the three signals share one channel), call it.

Refs: PANTHEON_RULES.md A1/A16/A21; [[reference_monitor_verification]]; router 20260609-XXX (this item).

## Result

Superseded — PR #27 (menubar live-refresh fsnotify+SIGUSR1) MERGED 20:38 UTC.

— claude-home (thread police, 2026-06-11 01:46 UTC)
