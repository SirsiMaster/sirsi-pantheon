# ADR-054 — One Horus: the unified agent fabric (identity & lifecycle, messages, work, models)

**Status:** DRAFT — owner-directed 2026-08-05 (claude-nexus, sne-50); review: codex-home.
**Supersedes nothing; composes:** ADR-051 (event-woken A2A conduit), the Router
Universal Task Ledger, `sirsi-inference/docs/design/MODEL-ROUTER-DESIGN.md`,
ADR-039 (continuous work surface), ADR-040 (verify-before-kill), ADR-043
(stray-thread reap).

## Owner directive (the mandate, near-verbatim)

Unify the Idea Router, the task ledger, and the model router as three
services under one aegis; everything requiring local agent LLM operations
sits on SNE. And the crux: *"there must be a mechanism somewhere where you
can actually ping/wake a thread… if threads can't be woken, restarted, or
reconstituted to pick up work, collect their task, or unregistered and
archived and demitted, it isn't worth all the tokens in the world."* Unless
a lane's PRD is complete and its task list exhausted, all available
resources are consumed toward goals.

## Decision

One service identity — **Horus** — over ONE durable store and ONE enforced
registry, exposing three planes. Beneath all three sits the layer the owner
correctly named as the key unlock: **identity & lifecycle**, whose core is
the wake matrix.

### 0. The wake matrix (foundation — every mechanism is real today)

| Lane shape | Wake mechanism | Status | Notes |
|---|---|---|---|
| codex-* headless CLI | launchd cli-spawn adapter; `ctr` triggers it on open items | **LIVE** — observed waking codex-io repeatedly | fresh process boots, reads inbox+ledger, works, exits |
| claude-* pinned interactive (CCD) | **session message injection** — a message sent into the running session arrives as a turn and the agent processes it | **LIVE — proven 2026-08-05**: the 43h-idle claude-pantheon lane was woken this way by Horus | the sanctioned "nudge, don't open sessions"; unavailable to/from unattended runs |
| claude-* headless routine | fresh `claude -p` session per wake, boots from ledger + thoth + continuation, exits (ADR-051 model) | designed, partially deployed | the owner's standing conversion directive for non-owner-facing lanes |
| any session, self-cadence | schedule/cron wakeups within the session runtime | LIVE | used for self-pacing only, never polling `ctr` |
| services (broker, dashboards) | launchd KeepAlive + fabric-watchdog with **real-completion probes** | LIVE | probes read the canonical port file; "healed" claims meet the same evidence bar as "healthy" |

Wake triggers are events, not timers: an item landing in an inbox, a task
assigned/unblocked, or a gone-stale projection (ADR-051's transactional
task events). Timers exist only as the supervision sweep's cadence.

### 1. Identity & lifecycle (enforced, not conventional)

REGISTER → BIND → WAKE → WORK → HEARTBEAT → EXIT/PARK → RECONSTITUTE → DEMIT.

- **REGISTER:** the store REJECTS send/pull/task operations from
  undeclared identities (sne-51). Registration names the lane, its wake
  mechanism from the matrix, its repo, and its workstream. This converts
  the Idea Router's inconsistency from a discipline problem into an
  impossibility.
- **BIND:** every thread binds to ledger tasks; tasks bind to a repo; the
  repo's PRD/canon is where tasks originate. An unbound thread is
  non-compliant and appears so on every surface.
- **RECONSTITUTE:** sessions are cattle; the STORE is the pet. Any lane can
  be rebuilt from durable state alone: ledger (what to do) + thoth (what is
  known) + continuation (where it stood). This is exercised reality — the
  fleet already resumes this way — the ADR makes it the contract: nothing
  a lane needs to resume may live only in a session.
- **DEMIT:** the orderly end: wake mechanism set to `none`, threads
  unregistered and archived, open tasks reassigned or closed, ledger
  history retained. (Exercised by claude-nexus PR #432; ADR-043 reaps the
  disorderly case.)

### 2. Three planes over one store

- **Messages (Idea Router):** ADR-051's conduit stands as-is — durable
  send/pull/close, revision-keyed task events, response contracts,
  owner-gate escalation cards.
- **Work (task ledger, schema v7 — sne-52):** each task carries its dynamic
  timeline (day/owner/hours), formal outline, charter, commission date,
  links to canon/repo/owner instructions, liveness, test-state,
  engineering stage, token consumption, duration. The ledger is the
  project-plan view; plans existing only in prose are non-compliant. The
  drill-down board (sne-53, Sirsi Nexus portal) renders it clickable
  end-to-end.
- **Models (model router):** per MODEL-ROUTER-DESIGN.md — policy engine,
  qualified-not-comparative lane selection, SNE as the local lane with
  real-completion liveness, remote adapters behind the same interface.
  **Every local agent judgment call (triage, summarization, insight)
  routes through this plane to SNE.** Deterministic router logic stays Go;
  SNE is the inference substrate, not the control plane.

### 3. Supervision (Horus duty, mechanical)

- The sweep keys on **task-touch age, never session heartbeats** — a
  breathing session with a stale registry is "alive but not working."
- Lane with unblocked open tasks quiet >4h → wake via the matrix +
  work-or-declare-blocked item, by name.
- Two failed wake cycles → owner escalation card (never silent).
- The live dashboard renders IDLE WITH WORK in red (shipped 2026-08-05);
  the portal board inherits it.
- Until the conduit hosts the scheduled sweep, supervision runs in every
  claude-nexus session and on every `ctr` (interim, declared honestly).

## Consequences

- A million ledgers are now worth their tokens: every row has a wake path,
  a reconstitution path, and a demit path.
- The registry becomes load-bearing: registration is the price of
  participation, enforced where it cannot be forgotten.
- SNE becomes the fleet's inference substrate in fact, not aspiration —
  and the Monday demo shows precisely that loop: task in ledger → routed
  via messages → agent woken → judgment on SNE → closure updates the
  board the owner is watching.

## Rollout (Monday demo path)

1. Wed: this ADR ratified (codex-home review, binding).
2. Wed–Thu: sne-51 store-boundary enforcement + sne-52 schema v7
   (codex-pantheon).
3. Thu–Fri: model router v1 + /api/ledger (claude-pantheon).
4. Fri–Sat: drill-down board in the portal (claude-nexus, sne-53).
5. Sun: end-to-end rehearsal (sne-54).

Non-goals: rewriting the store; replacing ADR-051 (it is the messages
plane); public A2A wire-format interop (a later adapter, not a
prerequisite).
