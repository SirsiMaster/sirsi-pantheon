# ADR-010: Pantheon Menu Bar Application

**Status**: Accepted ✅  
**Date**: 2026-03-25  
**Decision**: Build a native macOS menu bar application for Pantheon.  
**Implemented**: Session 18 — Phase 1 complete (headless + .app bundle)

## Context

Pantheon currently operates as a CLI-only tool. Users interact with it via terminal commands (`pantheon weigh`, `pantheon guard --watch`, etc.). However:

1. **Discoverability**: CLI tools are invisible — users forget to run them.
2. **Real-time monitoring**: The Sekhmet watchdog (`guard --watch`) runs in a terminal tab, not as a background service.
3. **IDE protection**: The Antigravity IPC bridge detects CPU starvation but has no way to alert the user visually.
4. **Professional presence**: A menu bar icon signals that Pantheon is active and protecting the machine.

## Decision

Build a **native macOS menu bar application** (NSStatusBarItem) that:

1. **Appears in the macOS menu bar** (top-right icon area)
2. **Visible in Finder** as a proper .app bundle in /Applications
3. **Runs Sekhmet watchdog** as a background daemon
4. **Shows real-time alerts** for CPU starvation, RAM pressure, ghost apps
5. **Provides quick actions**: scan, judge, guard, ka from the dropdown menu
6. **Integrates with Notification Center** for critical alerts

### Implementation Approach

**Option A — Go + systray library** (recommended for v1):
- Uses `github.com/getlantern/systray` or `fyne.io/systray`
- Single binary, cross-platform potential
- Menu bar icon + dropdown menu
- Spawns CLI subcommands in background

**Option B — Swift wrapper + Go backend** (future):
- SwiftUI for native macOS feel
- XPC or stdin/stdout IPC to Go binary
- Full Notification Center integration
- Requires Xcode and Swift toolchain

### .app Bundle Structure
```
Pantheon.app/
├── Contents/
│   ├── Info.plist          # Bundle metadata (CFBundleIdentifier, etc.)
│   ├── MacOS/
│   │   └── pantheon        # The Go binary
│   ├── Resources/
│   │   ├── AppIcon.icns    # Menu bar + Finder icon
│   │   └── Assets.car      # Asset catalog (future)
│   └── PkgInfo
└── Frameworks/             # (empty for now)
```

## Consequences

- Users get visual feedback that Pantheon is running
- Sekhmet watchdog becomes a persistent background guardian
- Antigravity alerts surface as native macOS notifications
- Pantheon becomes a "real app" visible in Finder and Launchpad
- Requires icon design and .app bundle packaging in the build pipeline

## References

- ADR-006: Self-Aware Resource Governance (Sekhmet watchdog)
- ADR-009: Injectable System Providers (Platform abstraction)
- `internal/guard/antigravity.go`: Alert ring buffer for UI consumption
- `internal/guard/watchdog.go`: Background monitoring engine
