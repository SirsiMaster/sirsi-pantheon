# ADR-057 — Operational enforcement lives in Pantheon's Go runtime, not in prompts

- **Status:** ACCEPTED (owner directive, 2026-08-06)
- **Supersedes in practice:** the prompt-level enforcement assumption in Rule A36
- **Owners:** claude-pantheon (runtime), claude-nexus (durable SNE/service owner), claude-home (Horus surface)

## Context — why A36 was not enough

Rule A36 ("Permanent Execution Loop") was landed on 2026-08-05 as text in `AGENTS.md`,
`CLAUDE.md`, and `PANTHEON_RULES.md`. It states that an agent may park only when its router
inbox, task ledger, and canon-completeness are simultaneously empty.

**That is a promise, not an invariant.** It is enforced by an agent choosing to honor a
sentence it read at session start. Nothing in the system detects a lane that stopped early,
nothing distinguishes a worker holding real work from one that merely looks alive, and
nothing prevents a `done` status that no evidence supports. A 15-minute heartbeat timer is a
bridge, not enforcement.

The owner's directive (2026-08-06): operational enforcement must live in the Go runtime.
Prompts, heartbeats, and AGENTS files cannot carry it.

This ADR is the design. It converts "never stop" from a promise into a system invariant.

## Decision

### 1. One authoritative work predicate

```
runnable =
    open router item
 OR actionable ledger task
 OR unmet traced canon requirement
```

A lane may park **only** when `runnable == false` across all three sources. This predicate is
computed by the runtime, not asserted by the agent.

### 2. Transactional work claims

Every worker follows:

```
pull → lease item → bind thread/task → execute → record evidence → close → pull again
```

Claims carry a **lease ID, expiry, heartbeat, attempt count, and idempotency key**. A worker
cannot silently "look active" while holding no task. Holding no lease while work exists is a
detectable, reportable state — not an invisible one.

### 3. Event-driven wake enforcement

Any of these durable store transitions emits a wake event:

- inbox item created or unblocked
- ledger task created, assigned, or unblocked
- a requirement audit creates an implementation gap
- a lease expires
- a worker completes an item while more work exists

Wake attempts require **durable IDs, bounded retries, acknowledgment through a real store
action, and terminal-failure escalation**. An acknowledgment is a store mutation, never a
process signal — a process that is running has not thereby acknowledged anything.

### 4. Honest supervision — Horus classifies every lane

| State | Meaning |
|---|---|
| `WORKING` | recent task-record mutation |
| `ASSIGNED` | actionable work, valid worker lease pending |
| `IDLE with work` | work exists but no recent store activity |
| `BLOCKED` | recorded technical/security/privacy blocker |
| `UNROUTABLE` | no valid wake path |
| `COMPLETE` | all three sources audited empty |

**Process existence and heartbeat prove session liveness only — never work.** This is the
green-surface-over-dead-thing class stated as a supervision contract: a live PID is not
evidence of progress, and any surface that treats it as such is lying by construction.

### 5. Mechanical reconciliation

A Go supervisor periodically compares:

```
router inbox ↔ task ledger ↔ thread leases ↔ requirement registry ↔ production evidence
```

It repairs stale registrations, expires orphan leases, wakes idle lanes, creates missing
ledger tasks, and **rejects inaccurate `done` statuses**.

### 6. Completion gate

`done` requires evidence references:

- requirement ID
- implementation commit/PR
- tests
- security control
- design acceptance
- deployment version
- production verification

**Whole-app completion is allowed only when every canonical requirement has a terminal
verified disposition.** A passing build, a merged PR, or a rendering screen is evidence of a
step and never of completion.

### 7. Codex-specific reality

Codex.app cannot guarantee endless execution from a text instruction alone. Pantheon needs an
**external Go supervisor** that wakes the registered Codex task when durable work appears,
verifies acknowledgment through a store action, and re-wakes after lease expiry. The current
15-minute heartbeat is a temporary timer-based bridge, explicitly not operational enforcement.

## Implementation sequence (minimum, ordered)

Each step is only useful once its predecessor exists; this order is a dependency chain, not a
preference.

1. **canonical requirement registry** — without it, "unmet traced canon requirement" in the
   predicate has no referent and the completion gate has nothing to trace to.
2. **durable leases and acknowledgments**
3. **three-source runnable predicate**
4. **event-triggered supervisor**
5. **stale/idle repair**
6. **evidence-backed completion gate**
7. **Horus operational surface**

## Consequences

- Rule A36 remains correct as a *statement of intent* but must stop being cited as the
  enforcement mechanism. A36's text should reference this ADR.
- Lanes that currently "look active" via heartbeat alone will begin reporting
  `IDLE with work`. That is the design working, not a regression — the state was always true
  and merely invisible.
- `done` will get harder to claim. That is the point.
- The 15-minute conduit timer becomes a fallback beneath the event-driven path rather than
  the primary mechanism.

## Numbering note

Taken as **ADR-057** because the 054-056 range is contested: `origin/main` carries **two**
distinct ADR-054 documents (`ADR-054-CONTRACTS-IDENTITY-AND-LEDGER-V7.md` and
`ADR-054-ONE-HORUS-UNIFIED-AGENT-FABRIC.md`) plus an ADR-055, while open PRs claim 051, 054,
055, and 056. PR #464 adds the ADR-uniqueness CI gate that makes this class detectable. If
that gate renumbers this range, this document moves with it.
