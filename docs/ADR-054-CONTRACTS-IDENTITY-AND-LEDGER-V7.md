# ADR-054 companion contracts — identity enforcement & ledger schema v7

**Status:** DECISIONS (claude-nexus, 2026-08-05) — unblocks codex-pantheon
`sne-51` and `sne-52`, which correctly blocked pending these. Binds with
ADR-054 (PR #495). Reviewer: codex-home.

---

## Part A — identity enforcement contract (unblocks sne-51)

codex-pantheon's open questions, answered decisively.

### A1. Validate SENDER, RECIPIENT, and TASK OWNER — all three

| Operation | Validated | On violation |
|---|---|---|
| `router send` | sender AND recipient | REJECT, exit non-zero, name which side failed |
| `router pull` / `close` / `respond` | the acting agent | REJECT |
| `task add` / `task update` | the task's owning agent AND `responsible_party` if set | REJECT |

Rationale for validating recipients too: an item addressed to an
unregistered id is born stranded — it can never be pulled. Today's `ctr`
proves the cost: items addressed to `owner` and `user` have been
accumulating for days as "agent not registered", visible but unreachable.
Rejecting at write time converts a silent permanent leak into an immediate,
fixable error.

### A2. Eligibility = DECLARED AGENT ONLY. Never live-thread.

A registered identity may be written to whether or not a thread is
currently alive. Requiring a live thread would destroy the offline-first
property that is the store's entire reason to exist (mail must wait for a
lane that is asleep, exited, or not yet reconstituted). Liveness belongs to
supervision and the wake matrix (ADR-054 §0/§3), never to admission.

"Declared" means present in `.agents/idea-router/agents.json` with: `id`,
`type`, `repo`, `workstream`, and a `wake` block naming a mechanism from
the ADR-054 wake matrix (`launchagent` | `session-message` | `routine` |
`none` | `owner-surface`). `wake.mechanism = "none"` is a valid declaration
(explicitly "do not wake me"), NOT an absence of one.

### A3. Register `owner` and `user` as first-class identities

Both are real destinations for escalation cards and both are currently
unregistered — the direct cause of the standing "not registered" errors.
Declare them with `type: human`, `wake.mechanism: "owner-surface"` (the
dashboard/menubar decision board, not a process). Then owner-gated items
route legitimately instead of erroring. `user` becomes an alias of `owner`;
new sends must use `owner`.

### A4. Fail-closed forward, fail-open backward

- New writes: rejected if any validated party is undeclared.
- Existing rows: remain readable, listable, closable forever. No
  retroactive invalidation — history is evidence.
- The enforcement switch ships ON by default. If a kill switch is wanted
  for rollout it must be a store state row (like `task_event_emission`),
  default enabled, and its disabled state must be visible on the board —
  never a silent env var.

### A5. Error text is part of the contract

A rejection must name: which party failed, that identity's id, the exact
file to declare it in, and the required fields. An enforcement error that
does not tell an agent how to become compliant will be routed around.

---

## Part B — ledger schema v7 contract (unblocks sne-52)

Migration lineage: **v4 → v7**, one migration, additive only. No existing
column changes type or meaning; every new column is nullable or defaulted so
v4 rows remain valid v7 rows. (v5/v6 were consumed by the ADR-051 conduit
work; v7 continues the sequence without reusing numbers.)

### B1. New columns

| Column | Type | Default | Units / enum | Update semantics |
|---|---|---|---|---|
| `charter` | TEXT | NULL | prose: why this task exists, its success condition | set at commission; amended only with a router item recorded in `links` |
| `commissioned_at` | TEXT (RFC3339 UTC) | task creation time | — | write-once; never rewritten |
| `commissioned_by` | TEXT | creating agent id | agent id or `owner` | write-once |
| `outline` | TEXT | NULL | markdown: the formal task outline | agent-owned; free to revise |
| `timeline` | TEXT (JSON array) | `[]` | `[{day,owner,hours,label}]`, hours = decimal hours | replace-whole-array on update |
| `links` | TEXT (JSON array) | `[]` | `[{kind,label,url}]`, kind ∈ `canon`\|`repo`\|`pr`\|`owner-instruction`\|`evidence` | append-preferred; dedupe by url |
| `liveness` | TEXT | `unknown` | `active`\|`stalled`\|`blocked`\|`unknown` | DERIVED each read — never stored stale (see B2) |
| `test_state` | TEXT | `untested` | `untested`\|`tested`\|`passed`\|`failed` | agent sets; `passed` REQUIRES a `links` entry of kind `evidence` |
| `stage` | TEXT | `spec` | `spec`\|`build`\|`review`\|`verify`\|`shipped` | monotonic forward; regression allowed but must state why in subject |
| `tokens_consumed` | INTEGER | 0 | output tokens, cumulative | additive increments only; never overwritten downward |
| `duration_seconds` | INTEGER | 0 | seconds of active work | additive increments only |

### B2. `liveness` is derived, not stored

Computed at read time from `updated` age and status, so it can never lie:
- `blocked` if `blocked_by` is set;
- `active` if status is `in-progress` and `updated` < 4h;
- `stalled` if status ∈ (`pending`,`in-progress`), no `blocked_by`, and
  `updated` ≥ 4h — this is the IDLE-WITH-WORK condition the owner's board
  renders red;
- `unknown` otherwise.
The 4h threshold is one constant, defined once, shared by store, CLI, and
every surface. Do not re-implement it per surface.

### B3. Accounting semantics

`tokens_consumed` and `duration_seconds` are **monotonic accumulators**
incremented by the working agent as it works (`task update --add-tokens N
--add-seconds N`). They answer "what did this cost", so they must never be
reset by a status change or a reassignment. On reassignment the accumulated
totals stay with the task, not the agent — the task is the cost center.

### B4. CLI surface (additive)

`task update` gains: `--charter`, `--outline @file`, `--timeline @file`,
`--link kind:label:url` (repeatable), `--test-state`, `--stage`,
`--add-tokens`, `--add-seconds`. `task list --json` returns every column
plus derived `liveness`. This JSON is the contract the drill-down board
(sne-53) renders; adding fields later is fine, renaming is not.

### B5. What v7 does NOT do

No table splits, no foreign keys to threads, no per-task history table. If
per-field audit history is wanted it is a v8 conversation, not a Monday one.

---

## Owner gate surfaced (not mine to close)

codex-pantheon also holds `pantheon-adr054-owner-gate`, blocked on **PR #465**
(public SNE claims/licensing canon) — open, non-mergeable, awaiting owner
content approval and rebase. Structural checks passed. It is correctly
owner-gated and is being surfaced on the owner's decision board rather than
resolved by any agent.


---

## Part C — one registration, every surface (owner directive 2026-08-05)

Owner: *"one registration should carry across all surfaces like OAuth."*

Registering an agent is ONE act with fan-out, not four chores an agent can
half-complete. Today's audit found **15 of 23 agents** present in the router
but missing a thread record, a task ledger, or both — each a lane the boards
cannot see.

### C1. `sirsi agent register <id>` is the single entry point

One command, transactional, creating all of it or none of it:

1. **router identity** — `agents.json` entry with id, type, repo,
   workstream, and a `wake` block naming a mechanism from the ADR-054 wake
   matrix.
2. **thread record** — registered and heartbeat-capable immediately.
3. **task ledger** — the agent's registry exists (possibly empty, but
   PRESENT: "empty" and "absent" must be distinguishable, because empty
   means *nothing to do* while absent means *invisible*).
4. **board presence** — derived, no separate step: every surface reads the
   three above, so nothing can be registered in one place and missing in
   another.

Partial registration is the failure this replaces. If any step fails the
whole registration fails loudly, naming the step.

### C2. The token analogy, stated precisely

Like OAuth, the agent presents ONE identity and every surface honours it —
no per-surface enrolment. Unlike OAuth, there is no bearer secret and no
expiry: the store is local and the identity is a declaration, not a
credential. Do not build token issuance, refresh, or scopes into this; the
security boundary is the machine, and inventing a credential system here
would add attack surface for zero benefit.

### C3. Demit is equally single-act

`sirsi agent demit <id>`: wake set to `none`, threads archived, open tasks
reassigned or closed, ledger history retained. A lane leaves cleanly and
completely, or it stays — never a phantom half-present in one surface.

### C4. Enforcement makes this self-correcting

With sne-51's store-boundary rejection live, an unregistered identity
cannot send, pull, or touch tasks at all. Registration stops being a thing
agents remember and becomes the price of participating — which is the only
version that survives contact with a fleet that has already drifted once.
