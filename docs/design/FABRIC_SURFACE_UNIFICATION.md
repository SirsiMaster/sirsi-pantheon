# Fabric surface unification — one contract, one vocabulary, three form factors

**Status:** SPEC — owner-directed 2026-08-06 ("why are there three surfaces
none of which agree? none of which look alike or impart the same
information. lets unify"). Binds: Sirsi Command Deck (menubar), the fleet
web board, and the Horus console.

## The actual defect (measured, not assumed)

The three surfaces were never disagreeing about one number. They render
**three different numbers under labels that sound interchangeable**:

| Surface | Displayed | What it actually counts |
|---|---|---|
| Command Deck | "81 pending" | open router **INBOX ITEMS** (messages) — 85 at time of writing |
| Fleet board (8734) | "60 in flight" | open **LEDGER TASKS** — 59 open of 207 total |
| Horus console (8080) | RAM / git / deities | **host health**; serves no fabric API at all |

"Pending" meaning messages on one surface and tasks on another is the whole
bug. A number without its stream named is a number that will be misread.

## Decision

### 1. One contract: `FabricBoard`

Every surface renders the SAME payload, produced by ONE producer
(`ledger.Build()` → `Summarize()`), never re-aggregated per surface:

```
FabricBoard {
  work:     { done, open, in_progress, assigned_not_started,
              stalled, blocked, total }        // LEDGER TASKS
  messages: { open, oldest_age, stranded }     // ROUTER INBOX ITEMS
  lanes:    [ { agent, state, open, active, stalled, blocked,
                last_touch, registered{router,thread,ledger} } ]
  health:   { ram_pct, swap, git_dirty, engine, issues }  // HOST
  activity: [ { at, agent, task_id, from, to } ]          // REAL transitions
  build, generated_at
}
```

Rules that make it non-negotiable:
- **Streams are never merged into one headline number.** Work and messages
  are separate objects because they are separate streams.
- **Every label names its stream.** "81 messages pending", never "81
  pending". "60 tasks in flight", never "60 in flight".
- `liveness` and lane `state` are DERIVED by the producer at read time
  (ADR-054 contracts §B2), never recomputed per surface — that is how two
  surfaces silently diverge.

### 2. One vocabulary + visual language

Identical wording and colour on every surface:

| Concept | Label | Colour |
|---|---|---|
| finished work | **Completed / in flight** | `#1D9E75` |
| work being done | **In progress / assigned** | `#EF9F27` |
| not moving | **Stalled / blocked / idle** | `#D85A30` |
| unread mail | **Messages pending** | neutral |
| lane doing work | **WORKING** | `#1D9E75` |
| lane with unblocked work gone quiet | **IDLE with work** | `#D85A30` |

No surface invents a synonym ("queued", "backlog", "waiting") for a concept
that already has a name here.

### 3. Three form factors, same information hierarchy

Order is identical everywhere so the eye learns one layout:
**headline three → needs attention → lanes → detail.**

- **Command Deck (menubar):** headline three + needs-attention only.
  Compact by necessity, but the three numbers must be the SAME three, with
  the same words, plus `messages pending` labelled as messages.
- **Fleet board (web, 8734 → portal):** the full board — headline three,
  activity feed, lanes, open task list, registration gaps, drill-down.
- **Horus console (8080):** host health is its own plane and stays; it
  ADDS the same headline three at the top so a fabric glance is available
  wherever the owner is looking. It must not compute them itself — it
  fetches `FabricBoard` like every other surface.

### 4. Producer and endpoints

`GET /api/fabric` returns `FabricBoard`, served by Pantheon (the data
owner, A26) on the dashboard port. Every surface consumes it:
- menubar: in-process `ledger.Build()` (same code path, no HTTP hop);
- web board and Horus console: HTTP `GET /api/fabric`;
- Nexus portal: same endpoint, falling back to the Nexus-side producer only
  while Pantheon's is unavailable, and never presenting a fallback number
  as authoritative without saying so.

**No surface may render zeros when its producer is unreachable.** It shows
a stale-data banner with the age of the last good payload. A dashboard that
renders zeros as data is worse than one that renders nothing (this failed
in production on 2026-08-05).

## Ownership

- Pantheon (codex-pantheon): `FabricBoard` producer + `/api/fabric` +
  menubar rendering.
- Nexus (claude-nexus): portal board + the 8734 reference implementation,
  which converges onto `/api/fabric` and is retired once the portal reaches
  parity.
- Horus console: adds the headline three, fetched not computed.

## Non-goals

Not merging the surfaces into one app — the menubar, the web board and the
console have genuinely different form factors and moments of use. Unifying
means one contract and one vocabulary, not one window.
