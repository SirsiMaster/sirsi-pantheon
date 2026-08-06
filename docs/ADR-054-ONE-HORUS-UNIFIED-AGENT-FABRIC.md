# ADR-054 — One Horus: the unified agent fabric (identity & lifecycle, messages, work, models)

**Status:** ACCEPTED — 2026-08-06. Owner-directed 2026-08-05 (claude-nexus,
sne-50). Reviewed by codex-home across four rounds; the eight binding
findings from head `b01b3f1` are resolved in this revision (see
"Findings resolved" below). The continuous-execution mechanism is amended
and mechanically enforced by ADR-057.

**Supersedes nothing; composes:** **ADR-052** (A2A/Router-Conduit Operating
Rules — Pantheon's adoption of the cross-portfolio conduit standard) and
**ADR-053** (Horus as per-node conduit: router + observability are one
flow). Cross-repo references to "ADR-051 A2A" resolve to ADR-052 in this
repo's ADR space; Pantheon ADR-051 is the SNE Supervisor Configuration
split and is NOT the conduit ADR. Also composes the Router Universal Task
Ledger, `sirsi-inference/docs/design/MODEL-ROUTER-DESIGN.md`, ADR-039
(continuous work surface), ADR-040 (verify-before-kill), ADR-043
(stray-thread reap).

**ADR numbering:** ADR-054 is this document and its companion contracts.
The SNE licensing/canon ADR that briefly also claimed 054 was renumbered to
**ADR-055** (PR #465). Next available: ADR-056.

## Owner directive (the mandate, near-verbatim)

Unify the Idea Router, the task ledger, and the model router as three
services under one aegis; everything requiring local agent LLM operations
sits on SNE. And the crux: *"there must be a mechanism somewhere where you
can actually ping/wake a thread… if threads can't be woken, restarted, or
reconstituted to pick up work, collect their task, or unregistered and
archived and demitted, it isn't worth all the tokens in the world."* Unless
a lane's PRD is complete and its task list exhausted, all available
resources are consumed toward goals.

## Findings resolved (codex-home binding review, head b01b3f1)

1. **ADR/canon collision with PR #465** — resolved: #465 renumbered to
   ADR-055, index and changelog updated there; this document keeps 054.
2. **Conduit lineage misattributed to ADR-051** — corrected above: the
   conduit lineage is **ADR-052 + ADR-053**. Every in-document reference
   is updated.
3. **Lean #9 close/respond ambiguity** — reconciled in §1a below.
4. **"ONE durable store" vs separately persisted model-plane state** —
   corrected in §2; the claim is now scoped truthfully.
5. **Heartbeat / current-item / process truth and duplicate dispatch
   authority** — reconciled in §3a.
6. **"All available resources" vs ADR-051 resource/yield safety** —
   reconciled in §3b.
7. **Wake matrix lacked evidence IDs, retry/backoff, acknowledgment,
   terminal failure, Lean #11 proof semantics** — added in §0a.
8. **Ratification mechanics** — status is ACCEPTED, ADR-INDEX and
   CHANGELOG updated in this change set, binding-hold satisfied at the
   reviewed head.

## Decision

One service identity — **Horus** — over ONE durable store and ONE enforced
registry, exposing three planes. Beneath all three sits the layer the owner
correctly named as the key unlock: **identity & lifecycle**, whose core is
the wake matrix.

### 0. Provider-neutral wake adapter contract

The matrix below records observed deployments; it is not the architectural
contract. The contract is capability-based and enforced by the Go runtime. An
adapter declares how to reach a worker and must support readiness probing,
bounded invocation, durable delivery identity, acknowledgment by an exact
source-store claim, lease renewal, and reconstitution. Provider or product
names never alter the lifecycle state machine.

| Lane shape | Wake mechanism | Status | Notes |
|---|---|---|---|
| codex-* headless CLI | launchd cli-spawn adapter; `ctr` triggers it on open items | **LIVE** — observed waking codex-io repeatedly | fresh process boots, reads inbox+ledger, works, exits |
| claude-* pinned interactive (CCD) | **session message injection** — a message sent into the running session arrives as a turn and the agent processes it | **LIVE — proven 2026-08-05**: the 43h-idle claude-pantheon lane was woken this way by Horus | the sanctioned "nudge, don't open sessions"; unavailable to/from unattended runs |
| claude-* headless routine | fresh `claude -p` session per wake, boots from ledger + thoth + continuation, exits (ADR-051 model) | designed, partially deployed | the owner's standing conversion directive for non-owner-facing lanes |
| any session, self-cadence | schedule/cron wakeups within the session runtime | advisory only | may accelerate a wake; cannot enforce continuity or acknowledge work |
| services (broker, dashboards) | launchd KeepAlive + fabric-watchdog with **real-completion probes** | LIVE | probes read the canonical port file; "healed" claims meet the same evidence bar as "healthy" |

### 0a. Wake delivery semantics (finding 7)

A wake is not fire-and-forget. Every wake attempt carries:

| Property | Rule |
|---|---|
| **Evidence id** | Each attempt writes a durable `wake_attempt` row keyed `(agent, trigger_event_id, attempt_n)`. "I woke it" is only claimable by citing that id — Lean #11 (Do No Harm) proof semantics: no claim without a record that survives the session. |
| **Acknowledgment** | A wake is ACKNOWLEDGED only when the woken lane performs an observable act against the store (pull, task touch, or close) after the attempt timestamp. A process starting is NOT an acknowledgment. |
| **Retry/backoff** | Unacknowledged attempts retry at 1m, 5m, 15m (three attempts). Backoff is per (agent, trigger), never a global timer, so one noisy lane cannot starve another. |
| **Terminal failure** | After the third unacknowledged attempt the wake is TERMINAL-FAILED: the attempt row is marked, supervision stops retrying, and an owner decision card is raised naming the lane, the trigger, and all three attempt ids. Terminal failure is never silent and never retried forever. |
| **Not-wakeable** | `wake.mechanism = "none"` short-circuits to TERMINAL-FAILED on first attempt with reason `declared-not-wakeable` — a declaration, not an error. |

Wake triggers are durable store events, not timers or prompts: an item landing
or becoming unblocked, a task being created, assigned, or unblocked, a
requirement audit creating a gap, lease expiry, or completion while more work
remains. Timers exist only for reconciliation and retry cadence.

### 1a. Lean #9 reconciliation — who may close or respond (finding 3)

Lean #9 ("Always Push & Verify") makes the acting agent responsible for
verifying its own writes land. That created an ambiguity against the
enforcement contract: may an agent close or respond on an item addressed to
a DIFFERENT agent?

**Amendment, binding:** close/respond admission validates the ACTING agent
only — it must be declared — and does NOT require the actor to be the
item's recipient. Supervision (Horus) and reviewers legitimately close
items they did not receive; requiring recipient-identity would make
sweeping stale records impossible. What is required instead: any close by a
non-recipient MUST record the acting identity in the result, so the ledger
shows who closed what on whose behalf. Lean #9's verify obligation attaches
to the actor, not to the addressee.

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

### 2. Three planes over one store — scoped truthfully (finding 4)

The store is the single authority for **messages, work, and identity**. It
is NOT the only persisted state in the fabric: the models plane keeps its
own operational state (provider credentials/config, budget and rate
counters, routing decision logs) outside the router store, and the SNE
engine keeps its own runtime state entirely. The accurate claim is:

> ONE durable store for the fabric's messages, work and identity; the model
> plane persists its own operational state separately and is joined by
> reference, not by shared tables.

Anything asserting a single store for *all* fabric state is wrong and this
ADR does not assert it.



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

### 3b. Resource posture — reconciled with ADR-051 (finding 6)

The owner directive "consume all available resources" and ADR-051's
resource/yield safety semantics are reconciled as: **saturate with WORK,
never with CONTENTION.** Concretely — supervision may wake any number of
lanes and keep every lane busy, but it must not (a) exceed the profile
concurrency caps the supervisor assigns (Anubis `interactive`, Ra `fleet`),
(b) start GPU/model work while a quiet-host window is declared, or (c)
override a lane's declared yield posture. An idle lane with unblocked work
is a leak; a lane thrashing against another lane's resources is a worse
leak. Where the two rules appear to conflict, ADR-051's safety semantics
win and supervision records why it yielded.

### 3. Supervision (Horus duty, mechanical)

- **Three truths, one precedence order (finding 5).** Heartbeat, current-item
  and process-liveness disagree routinely, so precedence is fixed: (1) a
  TASK-RECORD touch is the strongest evidence of work; (2) a store act
  (pull/close/respond) is next; (3) heartbeat and process-alive are
  liveness-of-the-SESSION only and never evidence of WORK. There is exactly
  one dispatch authority — the store — and any surface that appears to
  dispatch (menubar, dashboard, CLI) is a VIEW that must route its writes
  through the same facade; no surface holds a second authority.
- The sweep keys on **task-touch age, never session heartbeats** — a
  breathing session with a stale registry is "alive but not working."
- Any runnable lane without recent source-store mutation is IDLE WITH WORK and
  is woken immediately through its declared adapter; there is no four-hour grace
  period.
- Delivery is bounded and retried durably. Retry exhaustion becomes a terminal
  wake failure and an explicit escalation; it never disappears into a log.
- The dashboard read model can render IDLE WITH WORK in red; installed-runtime and cross-surface acceptance remain open under ADR-057.
  the portal board inherits it.
- The resident Go Horus supervisor is the enforcement authority. Session loops,
  prompts, heartbeats, and manual `ctr` invocations are observations or recovery
  edges only and cannot satisfy this duty.

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
