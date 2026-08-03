# Ledger Board — Canonical Component Spec

**Status:** Canon  
**Owner directive:** 2026-08-03 — "that visualized ledger is exactly what i'd like to see in Pantheon and Nexus builds"  
**Coordinated by:** claude-pantheon (cross-repo: Pantheon surfaces + Nexus adoption)  
**Refs:** ADR-014-STELE-LEDGER.md, ADR-026-HORUS-OPS-DASHBOARD.md, ADR-027-ROUTER-MENUBAR-SURFACE.md  
**Non-negotiables:** A32 (Owner Reporting Standard), A33 (live data only, no cached boards)

---

## Purpose

One board. Every Sirsi surface. The Ledger Board translates the task registry
into a human-readable chart-first status view: a hero completion percentage,
a stacked status bar, phase groups, a live blocked list (every row names its
gate), and action buttons whose labels ARE the next commands. It is not a
summary of the board — it IS the board.

This spec is the single source of truth so menubar, TUI, SwiftUI app, and
Nexus web/app all render the same layout without diverging into one-offs.

---

## Data Contract

Data source: `sirsi router ledger <agent> --json` (or the task registry
directly). **Never mocked. Never cached beyond one poll interval.**

```json
{
  "agent": "claude-pantheon",
  "tasks": [
    {
      "id": "task-id",
      "subject": "plain-English description",
      "status": "pending | in-progress | done | blocked",
      "responsible_party": "self | owner | <agent-id>",
      "blocked_by": "human-readable gate, e.g. 'waiting on sne-27' or 'waiting on you'"
    }
  ],
  "open_items": [
    {
      "id": "item-id",
      "from": "<agent>",
      "type": "proposal | decision | review",
      "age_seconds": 3600,
      "stale": false
    }
  ]
}
```

Derived fields computed client-side before rendering:

| Field | Derivation |
|---|---|
| `pct_complete` | `done_count / total_tasks * 100` |
| `in_flight_count` | tasks where `status == "in-progress"` |
| `blocked_count` | tasks where `status == "blocked"` |
| `stale_blocked_count` | blocked tasks where age > 48 h |
| `phase_groups` | surface-defined grouping of tasks by phase (see §Phase Groups) |

---

## Layout — Top to Bottom

All surfaces follow this vertical hierarchy. Terminal surfaces use
ASCII/box-drawing equivalents. GUI surfaces use the visual language below.

### 1. Headline Stat Row (3–4 cards)

Four cards in a row:

| Card | Label | Value | Color condition |
|---|---|---|---|
| Total | "tasks" | integer count | always neutral |
| Complete | "complete" | **hero %** (e.g. 73%) | green when 100% |
| In flight | "in flight" | integer count | neutral |
| Blocked | "blocked" | integer count | **coral/red only if stale (>48 h)** |

A fresh block is normal engineering. Stale blocks are red flags.
The blocked card stays neutral gray until `stale_blocked_count > 0`.

### 2. Stacked Completion Bar

One bar, full width. Fixed segment order (never reordered):

```
[████████████ done ████][████ in-review ███][████ queued ███][█ blocked █]
```

| Segment | Color | Hex | Notes |
|---|---|---|---|
| done | green | `#1D9E75` | leftmost |
| in-review | amber | `#EF9F27` | |
| queued | neutral gray | `#8B8B8B` | |
| blocked | coral/red | `#D85A30` | rightmost |

Legend below the bar uses counts, not just color swatches:
`● done (12) ● in-review (3) ● queued (8) ● blocked (2)`

### 3. Phase Groups

Tasks are clustered into **plain-English phases** — not raw task IDs. A
non-specialist reads the board in one pass. Each phase renders as:

```
Infrastructure Setup    ██████████████░░░░░░  7 / 10
Surface Implementation  ████████░░░░░░░░░░░░  4 / 10
Cross-Repo Coordination ██░░░░░░░░░░░░░░░░░░  1 / 5
```

Phase names are defined per-agent in a config section of the ledger data
(or inferred from task metadata). Rules:
- No jargon. Replace "ADR hydration" with "Architecture Decisions".
- Maximum 4–5 phases; merge thin phases rather than showing singletons.
- Each phase bar shows `X / Y` count inline (right-aligned).

### 4. In-Flight List

Current `in-progress` tasks, one line each:

```
▸ feat/ledger-board-component-spec  claude-pantheon  [in progress]
▸ nexus-dashboard-adoption          coordinator       [in progress]
```

Columns: task subject · responsible party · status badge.
Maximum 5 lines; overflow shows "+ N more in flight".

### 5. Blocked List

**Every blocked row states its gate on the same line — never a bare "blocked" badge.**

```
⚠ sne-28 approval      → waiting on you          (4 d)   [owner-gate]
⚠ tui-proof review     → waiting on codex-pantheon (2 h)  [agent-gate]
```

Format: `⚠ <subject>  →  <blocked_by>  (<age>)  [<gate-type>]`

Gate types: `owner-gate` / `agent-gate` / `external` / `technical`.
Age renders in minutes/hours/days. Stale threshold: **48 h** (then row
turns coral, card turns red).

### 6. Action Row

1–3 buttons. Labels are literally the next command or action — not "View
Details", not a picker. Examples:

```
[ sirsi router ledger claude-pantheon ]  [ Approve sne-28 ]  [ Requeue TUI proof ]
```

Buttons are styled as primary actions. On terminal: rendered as clickable
links or keyboard shortcuts `[1] cmd1  [2] cmd2  [3] cmd3`.

---

## Visual Language (GUI Surfaces)

| Property | Value |
|---|---|
| **Status colors** | done `#1D9E75`, in-review `#EF9F27`, queued `#8B8B8B`, blocked `#D85A30` |
| **Hero number** | serif/display face, tabular figures, 44–48 pt |
| **Cards** | flat, no gradients or shadows, 12 pt radius, hairline borders |
| **Icons** | SF Symbols (outline style), 16–20 pt, always paired with text |
| **Typography body** | system sans, 13–14 pt, no all-caps labels |
| **Background** | system window background; do not add tinted panels |

The restraint is deliberate: native macOS/iOS chrome already has depth.
Over-decorating fights the platform.

---

## Surface Implementations

### Pantheon Menubar

- Dropdown panel, fixed width **~320 pt**.
- Refreshed on open + every **30 s** while open (not while collapsed).
- Shows headline row + stacked bar + phase groups + blocked list.
- Action row: 1–2 buttons max; a "Full board →" link opens the TUI or app.
- No in-flight list (too narrow); in-flight count already in headline.

### Pantheon TUI

Terminal-native equivalent of the same hierarchy. Uses box-drawing and
ASCII progress bars (width = terminal width − 4):

```
╔══════════════════════════════════════════════════════╗
║  𓂀 claude-pantheon — Build Ledger                  ║
╠══════════════════════════════════════════════════════╣
║  Tasks: 25   Complete: 73%   In flight: 3  Blocked: 1║
╠══════════════════════════════════════════════════════╣
║  ████████████████░░░░░░  done(18) review(3) q(3) b(1)║
╠══════════════════════════════════════════════════════╣
║  Infrastructure Setup    ███████████░░░░  7/10       ║
║  Surface Implementation  ████░░░░░░░░░░  4/10        ║
║  Cross-Repo              ██░░░░░░░░░░░░  1/5         ║
╠══════════════════════════════════════════════════════╣
║  ▸ ledger-board-spec     self           in-progress  ║
╠══════════════════════════════════════════════════════╣
║  ⚠ tui-proof review  → waiting on you  (2 h) owner  ║
╠══════════════════════════════════════════════════════╣
║  [1] sirsi router ledger  [2] Approve  [3] Requeue   ║
╚══════════════════════════════════════════════════════╝
```

Progress bar char set: `█` (filled), `░` (empty). No color codes if
`--no-color` / dumb terminal; fall back to `#`/`.`.

### Pantheon SwiftUI App

First-class tab in the Horus dashboard. Live-updating via the ledger API.
Full layout: all 6 sections above, full width, scrollable if > ~600 pt.
SwiftUI: `VStack` + `LazyVStack` for phase groups, `GeometryReader` for
the stacked bar width, `.refreshable` on the scroll view.

Update cadence: foreground = every 15 s; background = paused.

### Nexus (SirsiNexusApp) — Operator Dashboard

Same component, same data contract, same visual language. Nexus manages
infrastructure; the fleet's task ledger IS infrastructure state, so the
board belongs in the operator dashboard as a first-class section.

Differences only where the platform forces it:
- Web: React/Next.js component mirroring the SwiftUI layout.
- Colors: same hex values (CSS variables referencing the shared design
  token `--color-status-done`, etc.).
- Data source: REST endpoint wrapping `sirsi router ledger <agent> --json`.
- Update cadence: SSE or polling every 30 s.

Cross-repo coordination is tracked as a ledger row so it's visible on the
board itself.

---

## Non-Negotiables

1. **Live data only.** A surface that shows a stale/cached board is worse
   than no board — it creates false confidence. The poll interval is the
   only acceptable freshness window.

2. **Blocked rows always name the gate.** "blocked" with no explanation is
   prohibited. Every blocked row includes `blocked_by` in the same line.

3. **No jargon in phase names.** A non-specialist must read the board in
   one pass, without asking what the phases mean.

4. **Color is never the only signal.** Every color badge is paired with a
   text label or count. Accessible at any contrast setting.

5. **Action buttons are commands, not navigation.** The label of an action
   button IS the thing it will do — copy-pasteable into a terminal, or
   directly executable on click.

---

## ADR Cross-References

| ADR | Title | Relevance |
|---|---|---|
| ADR-014 | Stele Ledger | Ledger data model and polling contract |
| ADR-026 | Horus Ops Dashboard | Dashboard architecture and tab layout |
| ADR-027 | Router Menubar Surface | Menubar panel design and refresh cadence |

---

## Change Log

| Date | Change |
|---|---|
| 2026-08-03 | Initial spec landed (owner directive: "same board in every surface") |
