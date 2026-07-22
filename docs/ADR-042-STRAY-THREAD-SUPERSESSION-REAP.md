# ADR-042 — Stray-Thread Supersession Reap (one live watcher per surface)

**Status:** Accepted — owner directive 2026-07-22
**Canon:** PANTHEON_RULES A24 (ADR-024 One-Watcher-Per-Surface), A25 (ADR-025 Thoth-Gated Exit), A22 (ADR-022 OS-Truth Liveness), A32 (write-amplification hygiene)

## Context

The thread registry accreted **520 records for ~11 live processes** — 259 `suspended`,
215 `reaped`, 27 `closed`. 145 records belonged to `claude-home` for **one** live watcher
process. The owner: *"ghost threads are as real as ghost apps… Pantheon should be REAPING
them automatically but isn't."*

Root cause — four compounding gaps:

1. **The generator.** The `SessionEnd` hook runs `sirsi thread suspend --self` on **every**
   exit, including every 15-min scheduled conduit run. Each ephemeral session leaves a
   suspended tombstone.
2. **Suspends are reaper-immune** by design (ADR-025 / ADR-022): `ReapDeadThreads` skips
   `suspended` because its PID is *expected* gone and reaping would destroy recoverable state.
   Correct for a human's resumable session — wrong for an ephemeral watcher that will never be
   resumed.
3. **No supersession rule.** `ReapDeadThreads` enforces OS-truth on *active* records but never
   enforced ADR-024: when a live watcher already holds a surface, its duplicate records were
   invisible to the reaper.
4. **Prune is manual + age-based.** `PruneClosed` / `PruneStaleSuspended` exist but only run via
   an explicit `sirsi thread prune`; the 7-day suspended retention is far too slow for ~200
   tombstones/day of churn.

## Decision

Add **`ReapStrayThreads`** to the read-time integrity pass (runs on every `thread list` and
node-status read — no daemon, "the read IS the event"). It enforces the ADR-024 invariant by
**supersession**, keyed on `(agent, surface, machine)`:

- After `ReapDeadThreads` retires dead-PID actives, any group that still has a **live watcher**
  (non-suspended, non-terminal, PID alive by OS truth) has its **non-live siblings** — superseded
  suspends and leftover dead-PID ghosts — retired to `closed`.
- **ADR-025 preserved:** a suspended record is retired ONLY when a live sibling supersedes it.
  An un-superseded suspend (its group has NO live watcher) is a genuine resumable pause and is
  left untouched — the resume-later guarantee holds until a successor actually takes the surface.
- **OS-truth preserved:** a live PID is NEVER retired. If two real watchers race one surface, both
  survive until one dies; only non-live records are swept.

**Nothing lost (owner directive).** Every reap is checked against durable memory: before a stray
is retired, `straySalvage` inspects it for real continuation state (a non-boilerplate resume
prompt, owned open items, a thoth ref, or an in-flight current item) and, when present, inscribes
it to the **Stele ledger** (`stele.TypeThreadReap`) so the state is durably captured even as the
record is swept. Empty tombstones (the `"session ended"` boilerplate with no owned work) inscribe
nothing — the check still runs; it simply finds nothing to save. Owned open items are additionally
safe because the live successor shares the same agent inbox — they are recorded, never stranded.

The salvage decision (`straySalvage`) is split from the side effect (`inscribeStraySalvage`) per
Rule A16, so the predicate is unit-testable without touching the ledger.

## Consequences

- The moment a successor session registers a live watcher, every prior record for that surface is
  provably superseded and swept on the next read. A churny surface can no longer accrete hundreds
  of tombstones — the pile is bounded to ~(live surfaces).
- This largely obviates a `SessionEnd` close-not-suspend change (a follow-on, optional): suspends
  become self-cleaning once a successor is live.
- `suspended` remains non-prunable in isolation (ADR-025 intact); it is only ever swept *because a
  live watcher supersedes it*, never on age alone.

## Alternatives rejected

- **Age-based suspended prune on a schedule.** Blanket age can wrongly reap a human's recent
  resumable pause, and at 200/day churn the pile still spikes between runs. Supersession is safe
  (only reaps the provably-superseded) and continuous.
- **Reap in the menubar surface pass.** Left scoped to its own agent by design (ADR-024 §resident
  surfaces); the CLI/node-status paths already run frequently enough for continuous reaping.
