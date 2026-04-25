# Session: Dashboard Build
**Date:** 2026-04-22
**Version:** v0.17.0-alpha
**Agents:** Dashboard agent (this session) + Packaging agent (Apr 20-22)
**Binary:** `sirsi` 14MB stripped, `sirsi-menubar` with embedded dashboard

---

## What Was Built

### Dashboard Server (`internal/dashboard/` — 1,058 lines)
Self-contained HTTP dashboard at `localhost:9119`. Zero external dependencies — all HTML/CSS/JS inline, following Seba's `graph.go` pattern.

| File | Lines | Commit Status |
|---|---|---|
| `server.go` | 179 | Committed (d9c8d42, lint-fixed by packaging agent) |
| `api.go` | 133 | Committed (d9c8d42, lint-fixed by packaging agent) |
| `colors.go` | 23 | **Uncommitted** |
| `pages.go` | 723 | **Uncommitted** |

### CLI Command (`cmd/sirsi/` — 280 lines)
| File | Lines | Commit Status |
|---|---|---|
| `dashboard.go` | 113 | **Uncommitted** |
| `dashboard_stats.go` | 167 | **Uncommitted** |

### Modified Files
| File | Change | Commit Status |
|---|---|---|
| `cmd/sirsi/main.go` | Added `dashboardCmd` to root | **Uncommitted** |
| `cmd/sirsi-menubar/main.go` | Dashboard server startup, "Open Dashboard" menu item, shutdown | **Uncommitted** |
| `Makefile` | `-s -w` strip flags → 13MB release binary | Committed (packaging agent) |

---

## Dashboard Pages (7 routes, all HTTP 200 verified)

| Route | Content | Data Source | Polling |
|---|---|---|---|
| `/` | RAM, Git, deities, accelerator, recent activity, Ra status | `StatsFn` + `notify.Store` | 10s |
| `/scan` | Anubis scan/judge/clean findings | Stele JSONL | Static |
| `/ghosts` | Ka ghost hunt results | Stele JSONL | Static |
| `/guard` | RAM sparkline (Canvas), pressure gauge, alert history | `/api/stats` + `/api/notifications?source=isis` | 20s |
| `/notifications` | Filterable table (source, severity, text search) | `notify.Store` | Static |
| `/horus` | Code graph analysis history, symbol search | Stele JSONL | Static |
| `/vault` | FTS5 search interface, vault activity | Stele JSONL | Static |

### JSON APIs
| Endpoint | Params | Source |
|---|---|---|
| `/api/stats` | — | `StatsFn()` → raw JSON passthrough |
| `/api/notifications` | `limit`, `source`, `severity` | `notify.Store` queries |
| `/api/stele` | `limit`, `type` | Direct JSONL file read (newest first) |

---

## Architecture Decisions

1. **`StatsFn func() ([]byte, error)`** — Returns raw JSON bytes. Menubar marshals its `StatsSnapshot`; dashboard passes through. Zero type coupling, no circular imports.

2. **Direct Stele read** — Reads `~/.config/ra/stele.jsonl` directly, not via `stele.Reader` (which advances consumer offset). Dashboard is read-only.

3. **DOM-safe JS** — All dynamic rendering uses `document.createElement()` + `textContent`. Never `innerHTML` with API data. Security hook validated.

4. **Injectable browser-open (Rule A16/A21)** — `openBrowserFn` with `sync.RWMutex`-protected accessors. Exported as `SetOpenBrowserFn()` for test injection.

5. **Port 9119** — Fixed port with `platform.TryLock("dashboard")` singleton. Same pattern as menubar's `TryLock("menubar")`.

6. **20s guard polling** — User corrected from 5s. Overview at 10s.

7. **13MB stripped binary** — `make build` uses `-s -w` by default. `make build-debug` for full 20MB with DWARF. Dashboard adds ~1MB (net/http stdlib).

---

## NOT Done

### 1. TUI Enhancement
**Target:** `internal/output/tui.go`
- Add notifications panel (read from `notify.Store`)
- Add status bar showing active operations
- BubbleTea TUI works (`sirsi` with no args) but has no notification awareness

### 2. Streaming Output
**Target:** `cmd/sirsi-menubar/handlers.go`
- `ExecuteWithNotify()` is still fire-and-forget
- Prompt asked: run command → stream output to dashboard → toast on completion
- Approach: `/api/events` SSE endpoint, `sync`-protected ring buffer in `ExecuteWithNotify`, JS `EventSource` in overview page

### 3. Tests
**Target:** `internal/dashboard/dashboard_test.go`
- `httptest.NewServer` handler tests
- Nil data source graceful degradation
- API JSON format validation

### 4. Design Alignment
- Dashboard uses dark bg (`#06060F`) — matches Seba's graph.go
- Public `sirsi.ai/pantheon` uses emerald green bg (`#022c22`)
- Consider unifying via `ColorBg` in `colors.go`

---

## Packaging Agent Context (Apr 20-22, all committed)

- v0.17.0-alpha: 12 platform binaries on GitHub Releases
- macOS DMG: `scripts/build-dmg.sh` → 10MB
- iOS: 8 SwiftUI views + 3 WidgetKit widgets + PantheonCore.xcframework
- Android: 8 Compose screens + signed APK (70MB) + pantheon.aar
- CI green: Go 1.25, golangci-lint goinstall mode, ubuntu/macOS/Windows
- RTK + Vault folded into Thoth as tabbed sections
- All URLs canonical: `sirsi.ai/pantheon` (31 occurrences fixed)
- Swift 6: `vm_kernel_page_size` hardcoded 16384 (ARM64)
- Android: Gradle requires JDK 17, not 25
- D-U-N-S check for Apple Developer: May 15, 2026

## Lint Fixes Applied by Packaging Agent
- `api.go:27` — `_, _ = w.Write(data)` (errcheck)
- `server.go:143` — `SetOpenBrowserFn` exported (was unused lowercase)

---

## Key Files for Next Session
```
internal/dashboard/pages.go         — HTML generation (bulk of the work)
internal/dashboard/server.go        — Server lifecycle (already committed)
internal/dashboard/api.go           — JSON endpoints (already committed)
cmd/sirsi-menubar/main.go           — Menubar integration point
cmd/sirsi-menubar/handlers.go       — ExecuteWithNotify (streaming target)
internal/output/tui.go              — BubbleTea TUI (enhancement target)
internal/seba/graph.go              — Reference: self-contained HTML pattern
internal/notify/store.go            — Primary data source (SQLite)
internal/stele/stele.go             — Event ledger (JSONL)
```
