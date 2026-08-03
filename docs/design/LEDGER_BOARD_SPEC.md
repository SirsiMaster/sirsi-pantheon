# Ledger Board — Component Specification

**Status:** Owner-confirmed canonical spec (2026-08-03)  
**Scope:** Every Pantheon surface (menubar, TUI, SwiftUI app) and Nexus (SirsiNexusApp) operator dashboard  
**Data source:** `sirsi router ledger <agent> --json` + task registry — always live, never mocked or cached beyond one poll interval

---

## Why this exists

The owner directive: *"that visualized ledger is exactly what I'd like to see in Pantheon and Nexus builds."* A single spec here means every surface builds the same board — not divergent one-offs. Consistency is not a style choice; a user switching between the menubar and the web dashboard should see the same numbers without reconciling them mentally.

---

## Data Contract

Already live — no new backend work required.

```json
{
  "agent": "claude-pantheon",
  "tasks": [
    {
      "id": "task-id",
      "subject": "plain-English description",
      "status": "pending | in-progress | done | blocked",
      "responsible_party": "self | owner | other-agent",
      "blocked_by": "item-id or description, empty if not blocked"
    }
  ],
  "open_items": [
    {
      "id": "router-item-id",
      "from": "sender-agent",
      "type": "decision | review | proposal | item",
      "age_seconds": 54321,
      "stale": true
    }
  ]
}
```

Poll: `sirsi router ledger <agent> --json`  
Interval: on open + every 60s while the surface is visible (menubar panel), live-updating (SwiftUI/Nexus), or on each render (TUI)

---

## Layout (top to bottom)

### 1. Headline Stat Row — 3–4 cards

| Card | Value | Color rule |
|------|-------|-----------|
| Total tasks | count | neutral |
| % Complete | hero number | green if ≥80%, amber if 50–79%, red if <50% |
| In-flight | count | neutral |
| Blocked | count | warning/danger color **only** if >0 for >48h (a fresh block is normal; a stale one is a red flag) |

The % complete number is the hero: serif/display typeface, tabular figures, ~44–48px in native surfaces, proportionally scaled in TUI.

### 2. Stacked Completion Bar — single bar, fixed segment order

```
[██████████░░░░░░░░▓▓▒▒] 
 done       in-rev  queue blocked
```

Fixed colors (see Visual Language below). Legend below the bar with counts, not color alone — never color-only encoding.

### 3. Phase Groups

Tasks clustered into plain-English phases (not raw task IDs). Each phase shows:
- Phase name (one human-readable line — no jargon, a non-specialist reads it in 5 seconds)
- Progress bar for that phase
- `X/Y` count

This is the translation layer between "45 tasks" and a stakeholder reading in one pass.

### 4. In-Flight List

Current `in-progress` tasks, one line each:

```
[task-id]  Subject text here           self
[task-id]  Another item description    owner
```

Columns: task ID, subject, responsible party. Truncate subject at ~60 chars in narrow surfaces.

### 5. Blocked List — non-negotiable gate naming

Every blocked row **must** name its gate on the same line:

```
[task-id]  Build ADR-052 branch   waiting on: owner (ADR number conflict)
[task-id]  Nexus ledger board     waiting on: claude-nexus (cross-repo coord)
```

**Never** render a bare "blocked" badge. The owner's explicit correction: *"just saying something is blocked because of me isn't effective."* If the gate is unknown, write "gate unclear — investigate" — at least that is honest.

### 6. Action Row — 1–3 labeled command buttons

Button labels **are** the next action, styled as primary actions — not links, not a picker:

```
[sirsi router pull --agent claude-pantheon]  [sirsi router ledger claude-pantheon]
```

In the menubar popover: 2–3 buttons. In TUI: rendered as key hints. In SwiftUI: tappable primary buttons. In Nexus: call-to-action row.

---

## Visual Language

Validated and consistent across all rendered surfaces:

| Status | Color | Hex |
|--------|-------|-----|
| done | teal-green | `#1D9E75` |
| in-review | amber | `#EF9F27` |
| queued | neutral gray | `#6B7280` |
| blocked | coral-red | `#D85A30` |

**Typography**
- Hero number: serif or display face, tabular figures, ~44–48px (native), proportional equivalent (TUI: large bold)
- Body: system default sans-serif, no custom fonts

**Cards**
- Flat, no gradients or drop shadows
- 12px corner radius
- Hairline border (`1px`, `rgba(0,0,0,0.08)`)
- Consistent with native macOS/iOS chrome — do not over-decorate

**Icons**
- Outline style (SF Symbols on Apple, Tabler-equivalent elsewhere)
- Never filled; always paired with text
- 16–20px; color alone never conveys status

---

## Surface-Specific Rendering

### Pantheon Menubar (macOS)

- Dropdown panel, ~320px wide
- Refreshed on open + every 60s while open
- Full layout above, scaled to panel width
- Hero stat and stacked bar visible without scrolling; phase list and blocked rows scroll inside the panel

### Pantheon TUI (terminal)

- Fixed-width text with box-drawing progress bars — the terminal-native equivalent of the same visual hierarchy
- Hero: large bold number with label
- Phase groups: `[████░░░░] Phase Name (3/8)`
- Blocked rows: indented with `⚠` prefix and gate text inline
- Renders in 80-column and wider; gracefully narrows

### Pantheon SwiftUI App (macOS, Phase 2)

- First-class tab or section, live-updating
- Full layout: all six sections above
- NavigationStack drill-in for task detail
- Matches ADR-030 native popover patterns

### Nexus (SirsiNexusApp) — Operator Dashboard

- Same board as a web/app component
- Nexus manages infrastructure, and the fleet's own task ledger IS infrastructure state — belongs in the operator dashboard verbatim
- Same data contract, same visual language, no separate design
- Coordination: claude-pantheon owns the spec; claude-nexus implements on the Nexus side (owner directive 2026-08-03)

---

## Non-Negotiables (A32/A33)

1. **Live data only.** A surface showing a stale or cached board is worse than no board — a green surface over a dead thing is the exact failure class this router effort exists to avoid.
2. **Blocked rows always name the gate.** Never a bare "blocked" badge.
3. **No jargon in phase names.** A non-specialist reads it in one pass.
4. **% complete is computed from the live task registry**, not from router item counts alone — items and tasks are distinct; conflating them produces a wrong number.
5. **Blocked count uses warning/danger color only if >0 for >48h.** A fresh block is expected engineering state. A stale one warrants attention.

---

## Implementation Checklist

For each new surface implementation, verify:

- [ ] Data sourced from `sirsi router ledger <agent> --json` (no mock, no cache >60s)
- [ ] All six layout sections present
- [ ] Stacked bar segments in fixed order: done / in-review / queued / blocked
- [ ] Every blocked row names its gate
- [ ] Phase names are plain English (no raw task IDs)
- [ ] Action row buttons labeled as literal next commands
- [ ] Blocked count warning color only triggers after 48h
- [ ] Hero number is visually prominent (display size, not body size)
- [ ] Status colors match the table above exactly

---

*Canonical source: this file. Updates require routing an item to claude-pantheon.*
