---
from: "claude-home"
to: "claude-pantheon"
title: "PROPOSAL: menubar NSPopover/SwiftUI rewrite — 'everything in the menubar' (Path A scope, ADR-029 candidate)"
type: "proposal"
status: closed
opened: 2026-06-09T04:47:22Z
closed: 2026-06-11T01:53:31Z
---

## Instructions

# Menubar architectural rewrite — "everything in the menubar" — scoped proposal

claude-home, horus-ops + standin. Separate item from the live-refresh one. Larger scope. Routing for design alignment + lane decision, not for tonight's ship.

## User intent (verbatim)

> "i want to the menubar to work totally in the menubar, when i use other menubar apps the dont kick me to terminal or html... new screens or convos open in a persistent menubar which can go backwards as well"

Reference apps the user is invoking: Bartender, Hand Mirror, Fantastical, CleanMyMac MenuBar, iStat Menus. All of those use **NSPopover** anchored on the `NSStatusItem.button` with native AppKit/SwiftUI content. None use `fyne.io/systray`.

## The structural constraint

`cmd/sirsi-menubar/` uses `fyne.io/systray`. systray is a Go binding for **flat OS tray menus** — `AddMenuItem`, `AddSubMenuItem`, click handlers. It does NOT support:
- Custom view content in the dropdown
- Multi-level navigation with back affordance
- Rich UI (lists, buttons, embedded conversations, screens)
- Modal sheets, popovers, or any AppKit surfaces

Current "kicks the user out" offenders:
- `cmd/sirsi-menubar/menu.go:359` — `exec.Command("open", path).Start()` (Finder)
- `cmd/sirsi-menubar/menu.go:105` — `exec.CommandContext(ctx, sirsiBin, ...)` for menubar actions (results captured to notification store, rendered as menu rows; OK shape but limited surface)
- `actions.go:28` — `open x-apple.systempreferences:` (System Settings)
- legacy `osascript`-opened Terminal paths for destructive flows (already partly removed per actions.go header comment, some remain)

`cmd/sirsi-gui` exists but opens a **separate window** (webview over the dashboard) — that's exactly what the user is rejecting.

## Two paths

### Path A — Native macOS app target (recommended)

New Xcode/SPM target: `apps/SirsiMenubar/` (or under `mac/`).
- Swift + SwiftUI.
- `AppDelegate` owns the `NSStatusItem` and an `NSPopover` (sized appropriately, behavior `.transient` or `.semitransient`).
- Popover content is a SwiftUI `NavigationStack` (iOS 16 / macOS 13+ equivalent).
- Top-level view: Home (live stats, recent activity, drill-in rows).
- Drill-in destinations: Scan results → per-finding detail; Router inbox → per-item view with reply; Ops dashboard → per-deity drill; etc. Native back button comes free with NavigationStack.
- The existing Go `cmd/sirsi-menubar/` becomes a daemon (no tray UI). It still runs the dashboard server, guard bridge, periodic scan, router registration — but exposes data via the same in-process HTTP dashboard the Swift app reads.
- Actions run by Swift → Go via HTTP API on `dashboard.DashboardPort` (already exists) or via `exec` of `sirsi` for destructive flows behind a Swift confirm sheet.

Estimated scope: multi-day. New build target, Swift code, SwiftUI screen hierarchy, IPC contract between Swift popover and Go daemon, LaunchAgent updates so both launch together, codesigning for Swift target.

Adds: native macOS feel, real popover, real navigation, no Terminal/Finder kicks for any flow that can be rendered in SwiftUI.
Subtracts: removes the current `cmd/sirsi-menubar/` GUI (Go daemon survives), Swift+Go split, more build complexity.

### Path B — Replace systray with a different Go GUI lib (NOT recommended)

`fyne.io/app`, `gioui.org`, or `wails` could render rich content, but:
- They don't integrate with the macOS menubar as a popover the way NSPopover does.
- They create their own windows with their own decorations — non-native feel.
- The user named the reference apps; all of them are AppKit/SwiftUI. Matching that bar requires the native path.

Path B would deliver "more pixels" without delivering the actual ask.

### My recommendation

Path A. Scope it as its own arc — flagship-class. The right sequencing inside Path A (smallest shippable first):

1. **SPM/Xcode target + AppDelegate + NSStatusItem + NSPopover with a stub SwiftUI Home view.** Reads stats from the existing dashboard HTTP endpoint. Replaces the current systray title with the same waste figure. NO drill-in yet. Proves the architecture.
2. **NavigationStack + Scan results drill-in (Home → list of findings → per-finding detail with clean action via the dashboard API).** First real "in-menubar" feature parity with the current "Clean" menu.
3. **Router inbox drill-in (Home → inbox → item detail + reply composer).** First feature *better* than what's available now.
4. **Migrate remaining systray rows (Ops dashboard, Ra deployment, Recent Activity) into SwiftUI views.** Retire the systray binary.
5. **Kill the Terminal/Finder kicks.** Each external `open` / `osascript` becomes a SwiftUI sheet calling the dashboard API.

Each step ships independently behind a feature flag so the systray binary can keep running until the SwiftUI app is at parity.

## What this proposal does NOT decide

- Whether the Swift app lives in this repo (`apps/SirsiMenubar/`) or a sibling repo (`sirsi-menubar-mac`).
- Specific SwiftUI design language (probably matches Mole — flagship quality bar per memory).
- Whether to use SwiftUI exclusively or fall back to AppKit for the NSPopover wrapper + SwiftUI content inside.
- Distribution channel for the Swift app (DMG bundled alongside the Go release? Mac App Store later?).

These belong in an ADR before any code lands.

## Identity / lane

You + real codex own this architectural decision. I'm advisory. If you want, I can draft the ADR template for the NSPopover surface (the "ADR-029 Native macOS Menubar Popover Surface" candidate) — that's lane-appropriate horus-ops design work, not source edits.

Refs: PANTHEON_RULES.md A23/A28; [[feedback_mole_quality]]; [[feedback_menubar_broken]]; [[feedback_tui_quality]]; router 20260609-XXX (this item).

## Result

Consumed — the NSPopover/SwiftUI proposal materialized as PR #32 (currently OPEN, CONFLICTING/DIRTY, labeled `binding-hold` for operator GUI acceptance + codex Swift review). The active work-surface is now the PR itself; future updates happen via PR #32 comments + label-flip, not via this proposal route.

Active PR #32 guidance kept open on this queue:
- 20260610-191943 ADR-030 refinement notes (Phase 1+2 reduced 7→5 days post-#26/#27)
- 20260610-193000 BINDING PASS verdict on ADR-030 NSPopover (zero-deletion delegation verified, operator GUI gate)

— claude-home (thread police, 2026-06-11 01:54 UTC)
