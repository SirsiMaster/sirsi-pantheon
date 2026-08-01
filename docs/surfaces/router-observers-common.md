# Router Observer Contracts — Common Source

> **Audience**: maintainers adding a new observer surface (board, doctor, menubar
> deck, dashboard, CTR, etc.) to the router read-model.
>
> **Authority**: this document is the shared contract source for all observer
> surfaces. Per-surface contracts (`router-board.md`, `router-doctor.md`, …)
> inherit these invariants and extend them with surface-specific obligations.

---

## 1. The Read-Model Contract (ADR-026)

`CollectNodeStatus` is the single aggregation point.  Surfaces MUST NOT
re-aggregate router state themselves: they receive a `NodeStatus` snapshot and
project it — they never re-read the items corpus, the thread registry, or the
work queue independently.

The contract is additive: new fields are backward-compatible; a surface that
does not yet read a new field is still correct.  Breaking changes (renames,
type changes) bump `SchemaVersion`.

---

## 2. Freshness — `generated_at` is the Data's Clock

`NodeStatus.GeneratedAt` (RFC3339) is when the snapshot was collected — the
data's own timestamp.  It is set **once**, in `CollectNodeStatus`, and never
mutated by a surface.

**Staleness rule**: observers MUST derive age from `generated_at`, not from
their own serving clock.  A cache that stamps its serving time tells the reader
*when they were answered*, not *how stale the answer is* — that is exactly
what ADR-003 forbids on the wire.

**Unknown age ranks worse than known-old**: a surface that holds a snapshot
with a missing or zero `generated_at` MUST refuse to present it as fresh.  A
known-old snapshot is better than an age-unknown one; render age explicitly,
not implicitly.

---

## 3. The Armed-Consumer Union

An agent is "armed" (has a live consumer watching its inbox) when **either** of
the following is true:

1. `AgentArmed(routerRoot, agentID)` returns `true` — at least one live,
   non-terminal, non-suspended thread is in the armed state (a `/loop` process
   or a fresh heartbeat, depending on `watcher_type`).
2. The per-agent launchd wake job `ai.sirsi.router.wake.<agentID>` is loaded
   (`launchctl list <label>` exits 0).

**Both legs must be checked.**  For an app-hosted session (e.g. a CLI agent
whose process is respawned each turn) there is no durable process to find; the
LaunchAgent is the only durable consumer it can have.  Crediting only the
registry thread check false-strands every wake-loop agent and misroutes its
peers (the 2026-07-31 D-DOC-1 incident: `claude-io` was reported loop-dead
while its `ai.sirsi.router.wake.claude-io` LaunchAgent was loaded and proved
responsive via `launchctl list`).

`computeStranded` already implements this union for stranded-inbox detection.
Every surface or predicate that classifies "armed" MUST apply the same union.

---

## 4. Error Visibility — Unknown is a Legitimate Value

An observer's empty or "all clear" state is a **positive claim**: "nothing
needs attention."  If a failure can produce that claim, the failure is
invisible precisely when it matters.

**An empty result after a failed read MUST be displayed differently from a
legitimately empty read.**  Positive claims ("0 open", "healthy", "none") are
only valid when the underlying read succeeded.

Surfaces MUST distinguish at least three states:

| State | Read outcome | Display |
|-------|-------------|---------|
| Data present | success | render the data |
| Legitimately empty | success, zero results | explicit "none — fabric healthy" or equivalent |
| Unreachable | error | explicit error / "unavailable" |

Collapsing "legitimately empty" and "unreachable" into the same display is a
reliability defect: the board's job is to say "nothing needs you right now,"
and a failure that silently produces that message is invisible precisely at the
moment it matters most.

---

## 5. Compliance Ratchet (ADR-002)

Surfaces not listed here are NOT out-of-compliance.  ADR-002's compliance
ratchet applies: **the next change to a surface brings that surface into
compliance** with the contracts in this document and the surface's own contract
file.  Speculative contracts for surfaces nobody is actively changing are the
ceremony ADR-005 §5 warns against.
