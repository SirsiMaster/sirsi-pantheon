# Ledger Board — Component Spec

**Status:** Canonical  
**Owner:** claude-pantheon (cross-surface coordination)  
**Confirmed:** Owner directive 2026-08-03  
**Refs:** ADR-050, ADR-051, A32, A33

---

## Purpose

The Ledger Board is the owner-facing task completion view rendered identically in every Pantheon surface (menubar, TUI, SwiftUI app) and in Nexus (SirsiNexusApp). It answers "what's the state of the fleet's work" in five seconds, without requiring the owner to grep router files.

---

## Data Contract

Live data only. Source: `sirsi router ledger [agent] --json` or `internal/ledger.Build()` directly.

```json
{
  "generated_at": "RFC3339",
  "stale_after": "4h0m0s",
  "agents": [{
    "agent": "claude-pantheon",
    "items": [{
      "id": "...", "title": "...", "from": "...", "type": "decision",
      "age_seconds": 3600, "stale": false, "picked": false,
      "blocked": false, "blocked_by": "", "dependency_chain": []
    }],
    "tasks": [{
      "agent": "claude-pantheon", "task_id": "nexus.ledger.1",
      "subject": "Land Ledger Board in Nexus", "status": "in-progress",
      "phase": "Cross-Repo", "responsible_party": "self",
      "blocked_by": "", "created": "...", "updated": "..."
    }],
    "oldest_item_age_seconds": 3600,
    "stale": false, "blocked_count": 0, "unblocked_unpicked_count": 1
  }]
}
```

`BoardSummary` (compact form for surfaces, produced by `ledger.Summarize()`):

```json
{
  "total_tasks": 12, "done_tasks": 7, "active_tasks": 4, "blocked_tasks": 1,
  "pct_done": 58,
  "phases": [
    {"name": "Infrastructure", "total": 5, "done": 4, "active": 1, "blocked": 0, "pct_done": 80},
    {"name": "Cross-Repo",     "total": 3, "done": 1, "active": 1, "blocked": 1, "pct_done": 33}
  ],
  "open_items": 3, "blocked_items": 1,
  "blockers": [{"agent": "claude-pantheon", "item_id": "...", "title": "...", "age": "2h30m"}]
}
```

A stale or cached board is worse than no board — surfaces must poll `ledger.Build()` on open and refresh on a bounded interval (menubar: every 60s while visible; TUI: on `u` keypress; SwiftUI: timer-driven).

---

## Layout — Top to Bottom

### 1. Headline stat row (3–4 chips)
| Chip | Value | Color |
|------|-------|-------|
| Total tasks | N | neutral |
| % Complete | N% | hero — large display face, tabular figures |
| In-flight | N | neutral |
| Blocked | N | **coral/red** only when >0 AND stale (>48h); amber when fresh |

### 2. Stacked completion bar
One bar, fixed segment order: `done / in-review / queued / blocked`. Legend below with counts.

Colors (consistent across all surfaces):
- done → `#1D9E75` (teal-green)
- in-review / active → `#EF9F27` (amber)
- queued / pending → neutral gray
- blocked → `#D85A30` (coral-red)

ASCII/TUI equivalent: `[████████░░░░░░░░░░░░]  58%`

### 3. Phase groups
Tasks clustered into plain-English phases (registered with `sirsi router task add --phase`). Each group shows a per-phase bar and `done/total` count. No jargon in phase names — a non-specialist reads it in one pass.

```
  Infrastructure    [████████░░░░]  4/5
  Cross-Repo        [████░░░░░░░░]  1/3
```

### 4. In-flight list
Current in-progress tasks, one line each: `task-id + subject + responsible party`.

### 5. Blocked list
Every blocked row states its gate in the same line — never a bare "blocked" badge.

```
  ⚠ [2h30m] Land Ledger Board in Nexus  ← waiting on codex-nexus
```

### 6. Action row
1–3 buttons whose labels are the next command, not abstract links.

---

## Visual Language

| Element | Value |
|---------|-------|
| Hero number | serif/display face, 44–48px, tabular figures |
| Cards | flat, no gradients/shadows, 12px radius, hairline borders |
| Icons | SF Symbols (native), Tabler-equivalent (web), outline only, 16–20px, always paired with text |
| Status colors | as above — consistent across ALL surfaces |

TUI surfaces use box-drawing characters and ANSI color codes to approximate the same visual hierarchy: hero %, phase groups, blocked-with-gate.

---

## Surface Rendering

### Pantheon menubar (`cmd/sirsi-menubar/`)
- Dropdown section "📋 Ledger — N% done"
- Sub-items: progress bar, done/active/blocked counts, blocked items with gate
- Refreshed every 60s in the background goroutine
- "View full ledger…" opens `sirsi router ledger` in Terminal

### Pantheon TUI (`internal/tui/`)
- Existing Activity screen (`screen_activity.go`) covers provenance; the Ledger Board is the task-registry view accessible via `sirsi router ledger` CLI
- Future: dedicated TUI screen (CmdScreenLedger) rendering `BoardSummary.TextBoard()`

### Pantheon SwiftUI app (`cmd/sirsi-gui/`)
- First-class tab "Ledger" with live-updating board
- Data via `sirsi router ledger --json` subprocess or embedded `ledger.Build()`
- Timer-based refresh (30s interval while tab active)

### Nexus (SirsiNexusApp)
- Same `BoardSummary` data contract, same visual language
- Rendered as a web/app component in the Nexus operator dashboard
- See `docs/design/NEXUS_LEDGER_COORDINATION.md` for integration seam
- No separate design — same component, same data, same colors

---

## Non-Negotiables (A32/A33)

1. **Live data only.** A surface showing stale/cached data is worse than no board.
2. **Blocked rows name the gate.** Never "blocked" alone.
3. **No jargon in phase names.** Non-specialist readable in one pass.
4. **Identical visual language** across all surfaces — divergent one-offs are a bug.
