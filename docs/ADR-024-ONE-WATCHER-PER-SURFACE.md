# ADR-024: One Watcher Per Surface — Router-Prescribed Heartbeat

## Status
**Accepted + Implemented** — June 1, 2026. Reviewed by codex-pantheon (`reviews/20260601-codex-pantheon-adr024-one-watcher-review.md`, verdict *approve-with-acceptance-edits* against commit `941d5a6`); the three acceptance edits and three operational findings below are folded in. Implementation (Go + hook) by claude-pantheon — commits `7c4cda5` (register handshake + watcher-spec table), `10c5e93` (§5 one inbox + typed items), `9288534` (supervisor hook: check-then-arm, caffeinator retired). All 7 acceptance tests below pass (Go + python, `go test -race` green). Remaining deploy step: migrate the hook to user-scope `~/.claude/` (Decision 4). codex verifies arch after.

## Context
A27 (Heartbeat Loop Mandate) says every registered thread must run a watcher
from `register` → `close`. It did not say **how many**, and the answer drifted to
"as many as got built." On 2026-06-01 a single `claude-home` thread was kept
alive by up to **three independent mechanisms**, each authored in a different
session blind to the others:

| Mechanism | Spawned by | Heartbeat | Wakes Claude conversation? |
| :--- | :--- | :--- | :--- |
| `watch-router` fs-watcher | `sirsi thread register` (auto) | yes (FSEvents) | **No** |
| Python caffeinator | `.claude/hooks/router_inbox_check.py` | 60s loop | No |
| `/loop` Monitor | the agent, manually | per-tick | **Yes** |

This is precisely the failure R4 (Capability-First) exists to prevent: a
primitive rebuilt from scratch because its existence was invisible. Worse, only
**one** of the three (the `/loop` Monitor) can actually wake a Claude
conversation — the other two keep the *record* warm but never deliver an inbox
item to the agent (the wake-asymmetry: file drops notify Codex's poller, not
Claude). So the redundant mechanisms cost CPU, Spotlight `mds_stores` pressure
(the same write-amplification implicated in the 2026-06-01 lockup), and registry
churn — while adding zero wake coverage.

The root defect is that **arming was guessed per surface**. `register`
unconditionally spawned its own fs-watcher; the hook spawned a caffeinator; the
agent armed `/loop` — nobody owned the decision of *what the correct single
watcher for this surface is*.

**Three further defects surfaced while dogfooding this ADR's own `/loop`
watcher on `claude-home` (2026-06-01) — each is folded into the Decision:**

- **F1 — watchers are not durable across wakeups.** A persistent `/loop`
  Monitor vanished across a `ScheduleWakeup` re-invocation, leaving the thread
  registered-but-unwatched (the exact A27 failure). "Arm once at register" is
  insufficient; the watcher must be **re-asserted idempotently** every
  SessionStart/wakeup.
- **F2 — liveness is OS truth, not harness truth.** The re-assertion check
  cannot key on the harness task list: `TaskList` reported "No tasks found"
  while the Monitor was provably alive, which led to arming a **duplicate**
  watcher (the very thing this ADR forbids). Idempotency must key on an
  OS-level signature — the same `(agent_id, pid)` truth ADR-022 uses for
  reaping. (This mirrors ADR-022: trust the OS, not a recency record.)
- **F3 — replies land in sibling directories the inbox reader ignores.** This
  very review was correctly addressed `to: claude-home` but written to
  `reviews/`, not `items/`. `sirsi router pull <agent>` and the `/loop` watcher
  scan only `items/`, so an addressed, open reply sat unread — the multi-agent
  relay (A26) silently stalled. The reader, not the sender, is wrong: codex
  addressed it properly.

## Decision

### 1. One liveness/wake mechanism per surface — invariant
A registered thread runs **exactly one liveness/wake mechanism** (codex edit 2 —
phrased this way, not "one watcher," so resident UI surfaces that only heartbeat
are not implied to be inbox workers). Only **agent-capable** surfaces
(`claude`, `codex`, `gemini`/`gemma`/`qwen`, headless) require that mechanism to
also watch `items/`; resident surfaces (`menubar`/`tui`/IDE/`macapp`) may
heartbeat without becoming inbox pollers (preserves ADR-020). The correct
mechanism is a function of the **surface**, not of which session got there first:

| Surface | Canonical watcher | Heartbeat | Watches inbox | Wakes agent |
| :--- | :--- | :--- | :--- | :--- |
| `claude` | `/loop` + file Monitor on `items/` | per-tick (≤60s) | yes | yes |
| `codex` | app heartbeat (`ctr-thread-wake`) | native poll | yes | yes |
| `gemini` `gemma` `qwen` | surface-native pull loop over `items/` | loop | yes | yes |
| `menubar` `tui` `vscode` `jetbrains` `cursor` `macapp` | native runloop ping | ≥60s, bounded | only if it acts on items | n/a (resident) |
| `mcp` `api` `webhook` `worker` | bounded headless pull loop over `items/` | loop | yes | n/a |

### 2. The router prescribes the watcher — register becomes a handshake
`sirsi thread register` **stops auto-spawning the fs-watcher.** Instead it
returns the prescribed watcher for the surface. A new `internal/router`
watcher-spec table (sourced once, consulted always — the R4 living inventory in
code) maps surface → `{type, mechanism, arm_instruction, heartbeat_interval}`.

`sirsi thread register --surface claude --json` returns, alongside `thread_id`:

```json
{
  "thread_id": "thr-…",
  "watcher": {
    "type": "loop-monitor",
    "mechanism": "/loop + Monitor on .agents/idea-router/items/",
    "arm_instruction": "Arm /loop watching items/ for `to: <agent>`; heartbeat each tick. Re-assert idempotently keyed on the thread_id (`pgrep -f thr-<thread_id>`) — NOT the shared loop body / `DIR=` string (it collides with other agents' loops on a shared host), NOT TaskList.",
    "heartbeat_interval_s": 60
  }
}
```

The thread's job is no longer "invent a watcher" but "arm the one the router
named." The router owns the mapping; the surface owns the arming. Re-register is
idempotent on `(agent_id, pid)` (A27) and returns the same spec.

### 3. The agent is *told* to arm — re-asserted every wakeup, keyed on OS truth (R2 enforcement)
The register handshake is the enforcement point. The SessionStart hook calls
`register --json` and **injects `watcher.arm_instruction` into the agent's
context**, so a Claude session is instructed to arm `/loop` the moment it
starts — closing the gap where R2 ("monitor armed") relied on the agent
remembering.

Arming is **check-then-arm, re-run on every SessionStart/wakeup, not once** (F1):
a `/loop` Monitor does not reliably survive a `ScheduleWakeup` re-invocation, so
the watcher must be re-asserted each time the thread wakes or the thread silently
becomes registered-but-unwatched.

The check is **idempotent on OS truth, never the harness task list** (F2):
re-arm only when **zero** matching watcher processes exist for this
`(agent_id, thread_id)` — detected by an OS signature that includes the
**thread_id** (`pgrep -f thr-<thread_id>`), the same `(agent_id, pid)`
identity ADR-022 reaps on. The signature MUST be the thread_id, **not** the
shared loop body or `DIR=.agents/idea-router/items` string — every Claude
surface runs the same body, so a shared-string `pgrep` matches *other agents'*
loops on the same host and falsely reports "already armed" (observed
2026-06-01: claude-deck's check matched claude-home's live loop). `TaskList`/
harness views may falsely report empty and MUST NOT be the arming gate, or they
cause duplicate watchers.

A `Stop`-hook gate (`exit 2` until the surface's watcher is detected alive via
the same OS signature) is the backstop for surfaces that ignore the instruction;
it is **off by default** and gated by `SIRSI_SUPERVISOR` (see §4).

### 4. Default-on, human off-switch
The supervisor is on by default via **user-scope** `~/.claude/settings.json`
hooks (not project-scope — the 2026-06-01 miss was caused by the hook living in
`sirsi-pantheon/.claude/`, so home-dir sessions never fired it). The single
off-switch is the env gate `SIRSI_SUPERVISOR=0`.

`SIRSI_SUPERVISOR=0` disables supervisor-**managed arming and Stop-gate
enforcement only** — `register` still returns the canonical watcher spec
(codex edit 1). The spec is the router's capability-inventory output (R4); a
diagnostic caller must be able to ask "what is the correct watcher for this
surface?" precisely when the supervisor is off. Off means "don't arm/enforce for
me," not "hide the answer."

### 5. One inbox — `items/`. Collapse the reply channels, don't chase them (F3)
The root cause of F3 is **too many places to respond**: `items/`, `reviews/`,
`decisions/` are three sibling drop-locations, and a reply addressed
`to: claude-home` in `reviews/` went unread because the reader only scans
`items/`. The lean fix is collapse, not a three-directory scan:

- **Every agent-to-agent message is an addressed item in `items/`** — proposal,
  review, and decision become a `type:` field on one item, not three
  directories. `sirsi router send` already writes there; `reviews/` and
  `decisions/` are **retired as reply channels** (kept, if at all, only as
  rendered/derived artifacts that an `items/` entry links to — never a place an
  agent must independently poll).
- The reader stays trivial: one inbox = open `to: <agent>` items in `items/`.
  No multi-directory union, no per-agent convention to remember.

This is the same disease the ADR treats (accretion of parallel mechanisms),
applied to message channels. One watcher per thread; one inbox per agent.

### 6. Adopted-process bridge — the one allowed fs-watcher (codex arch-verify ruling)
`sirsi thread discover` adopts an **already-running** local process that never
entered the register handshake — so the router has no channel to inject
`watcher.arm_instruction` into that live conversation. For such a process,
`discover` MAY spawn a **bounded fs-watcher as a named adoption bridge** — the
single sanctioned exception to "register does not spawn a watcher." Rules:

- It is an **adoption bridge, not the canonical watcher** for the surface (a
  `claude` process's canonical watcher is still `/loop`, self-armed).
- It is **anchored to the adopted PID** and removed when that process exits or
  the thread is closed.
- It is **never** used for sessions that self-register through the normal
  handshake.
- **Lifecycle handoff (required guard):** when a later self-register/re-register
  for the same `(agent_id, pid)` returns the canonical watcher spec, any existing
  adoption fs-watcher for that thread MUST be killed/superseded. Without this, a
  discovered Claude process can arm `/loop` *and* keep the bridge — recreating
  the duplicate-watcher accretion this ADR exists to kill.

## Consequences
- A thread can no longer accumulate redundant heartbeats: there is one watcher,
  named by the router, per surface. The caffeinator and the auto-spawned
  fs-watcher are **retired** for `claude` (the `/loop` Monitor subsumes both).
- `register` is no longer a side-effecting spawner; it is a pure handshake that
  *returns* what to do. Easier to test, no orphaned watcher processes when a
  thread is adopted under a new PID.
- Spotlight/`mds_stores` pressure from N redundant heartbeat loops drops to one
  bounded ping per live thread (ties to the write-amplification lockup fix).
- The surface→watcher table is the R4 capability inventory **in code** —
  consulted on every register, so no future session can rebuild a watcher
  without first being handed the canonical one.

## Deferred / follow-up
- Migrate `router_inbox_check.py` to user-scope and strip its caffeinator
  (superseded by the register handshake + `/loop`).
- `sirsi thread suspend` (resumable state carrying memory+plans) — A27 lifecycle
  completion, tracked separately.
- Reconcile or delete the stranded `internal/router/daemon.go` dispatcher code;
  the active CLI contract is pull-model and no daemon verb is exposed.

## Acceptance tests (codex edit 3 — required before merge; owner: claude-pantheon)
- `register --surface claude --json` returns a `watcher` block and does **not**
  call `spawnRouterWatcher` (no `/tmp/sirsi-router-watch-*.pid` for claude).
- `register --surface menubar --json` returns a native-runloop heartbeat spec
  with interval ≥60s and **no** inbox-worker requirement.
- Repeated `register` on the same `(agent_id, pid)` returns the same thread and
  the same watcher spec (idempotent).
- `SIRSI_SUPERVISOR=0` leaves the spec **visible** but suppresses managed arming
  and Stop-gate enforcement (§4).
- **F1**: after a simulated wakeup with the Monitor gone, check-then-arm
  re-asserts exactly one watcher.
- **F2**: with `TaskList` empty but an OS watcher alive, check-then-arm does
  **not** spawn a duplicate (gate keys on `pgrep`, not the task list).
- **F3**: a reply addressed `to: <agent>` is surfaced by `sirsi router pull`
  from `items/`; `reviews/`/`decisions/` are no longer polled as inboxes.

Refs: PANTHEON_RULES.md A27 (heartbeat loop), A26 (router relay), A24 (autonomy),
A11/A19 (Spotlight write-amplification context); companion to ADR-022 (CTR
OS-truth liveness). Resolves the three-heartbeat accretion observed 2026-06-01.

---

## Amendment 1 — Worker-lifecycle gate + reap-key identity (DRAFT)

**Status:** IMPLEMENTED — June 2, 2026. Sole writer: claude-pantheon (router CLAIM
`20260602-024522`, claude-home root-authority). Design APPROVED by claude-home
(`20260602-025217`). Covers registration-hygiene findings (2) and (3) from the
2026-06-01/02 CTR accretion sweep. Finding (1) — menubar non-idempotent
registration — stays with the surface-chrome source-lock holder per ruling
`20260602-023813` and is **not** in this amendment's scope. Implementation routed
to codex-pantheon for review-of-code (doer→reviewer); user directed driving to
completion. All acceptance tests below pass (`go test -race` green):
- (3) `internal/router/`: composite `PIDStateOf(pid, startedAt)` + `PIDRecycled`
  + `defaultPIDStart` (`ps -o lstart=`) in `liveness.go`; `Thread.StartTime`
  captured at register; `ReapDeadThreads` + `RegisterThread` fast-path keyed on
  the composite (`adr024_amend_test.go`). Adopted claude-home note (b): one
  canonical `PIDStateOf(pid, startedAt)`, `""` = legacy bare-PID.
- (2) `cmd/sirsi/`: injectable `oneShotProbe` + `ephemeralWorkerSkip`; `register`
  refuses one-shot `--print`/`-p` workers (no-op, not error). Selective-gate test
  (claude-home note a) proves an interactive surface registers under the same path.

### Context

Two registration defects survived the ADR-024 watcher consolidation and the
ADR-022 reaper, and both produce the same observable symptom — a registry that
either balloons with phantom `active` records or reaps live threads:

- **(2) Ephemeral workers register persistent threads.** Ra/router-dispatched
  `claude --print` workers (and the fs-watcher-spawned `ctr` workers ADR-024 §D2
  is retiring) are *one-shot, non-interactive* processes. When such a worker runs
  `sirsi thread register`, it mints a persistent CTR thread that outlives the
  worker by milliseconds — then the worker exits and the record is left for the
  reaper. At scale this is a dominant source of phantom records. ADR-024 already
  classifies surfaces (`watcherspec.go`), but registration itself has **no gate**
  preventing a non-interactive invocation from registering.

- **(3) The reaper keys on a bare PID.** `internal/router/liveness.go`
  (`PIDStateOf` → `ps -o stat=`) answers "does PID N exist and is it non-defunct"
  — but a PID is **not a stable identity**. The OS recycles PIDs, and a session
  that re-registers gets a *new* anchor PID while a prior record still names the
  old one. Bare-PID liveness therefore (a) reaps a live thread whose recorded PID
  was a short-lived intermediate that has since exited, and (b) in the reuse case
  could read a *recycled* PID belonging to an unrelated process as "alive,"
  resurrecting a stale record. **Observed live this session:** thread
  `thr-f78c30cf3088fea3` was reaped within ~60s (`PID 12080 gone`) because the
  register anchor resolved to a transient shell PID, not the long-lived agent
  process — the exact (a) failure. This is the highest-confidence systemic bug.

### Decision

**(2) Worker-lifecycle clause (A27 addendum, resident-surface gate).**
Registration of a *persistent* CTR thread is gated to **interactive / resident
surfaces** — the surfaces ADR-024's `watcherspec.go` already enumerates
(`claude`, `codex`, `gemini`/`gemma`/`qwen` interactive; resident `menubar`,
`tui`, `vscode`/`jetbrains`/`cursor`, `macapp`). Ephemeral Ra/router-dispatched
`claude --print` (or `-p`) workers and fs-watcher-spawned `ctr` workers **MUST
NOT** register persistent threads. This is an *interactive-surface gate, not an
any-invocation gate* (agreed option (a)): a worker may still read/act on the
router, but it does not enroll as a live node. Stated alongside A27's
resident-surface clause: *registration means "an interactive or resident surface
is alive and watching" — a one-shot worker is neither.* The signal already exists
in code (`isOneShotWorker` in `threaddiscover.go` detects `--print`/`-p`); the
clause makes refusing-to-register the rule, not just a discover-time skip.

**(3) Reap-key = composite identity, not bare PID (ADR-022 correction).**
A thread's OS-truth identity is **`(pid, start_time)`** — or equivalently
`(agent_id, pid, start_time)` — never bare `pid`. `start_time` (process boot
time) is the cheap generation discriminator: a recycled PID carries a *newer*
start time, so the composite key mismatches and the record reads `gone` rather
than being falsely revived; and a record whose recorded `(pid, start_time)` no
longer matches a live process is `gone` even if that bare PID was reused.

- **Discriminator:** `ps -o lstart= -p <pid>` (BSD/macOS) / `ps -o lstart=`
  (Linux) — a stable per-process boot timestamp, one cheap `ps` call (no new
  dependency, same shape as the existing `ps -o stat=` probe).
- **Capture:** `Thread` records `start_time` (the recorded process's `lstart`) at
  register time, alongside `pid`. Backward-compatible: a record with an empty
  `start_time` falls back to today's bare-PID behavior (no regression for
  pre-amendment records; they age out via prune).
- **Fix path (post-review):** extend `liveness.go` with a
  `PIDStateOfWithStart(pid int, startedAt string) PIDState` that returns `gone`
  when the live PID's `lstart` differs from the recorded one; `ReapDeadThreads`
  and `RegisterThread`'s idempotency fast-path key on the composite. Injectable
  per A16, mutex-guarded per A21, exactly like the current prober.

### Acceptance tests (required before merge; owner: claude-pantheon, post-review)

- A `--print`/`-p` worker invoking `thread register` is **refused** (or no-ops
  with a clear message) and creates no persistent record; an interactive/resident
  surface registers normally.
- `ReapDeadThreads` marks a record `reaped` when the live PID's `start_time`
  differs from the recorded one (recycled-PID case) — proven with an injected
  prober, no real processes (A16).
- A record whose `(pid, start_time)` still matches a live process is **not**
  reaped (no false-positive on a genuinely live thread — the regression that bit
  `thr-f78c30cf3088fea3`).
- A pre-amendment record with empty `start_time` retains today's bare-PID
  semantics (backward-compatible).

### Neith's Triad note (A22)

This is an amendment to an accepted ADR, not a new ADR; the parent ADR-024 and
companion ADR-022 carry the full Triad. The fix path above is the
"Recommended Implementation Order"; the `(pid, start_time)` vs bare-`pid` choice
is the "Key Decision" (rejected alternative: heartbeat-recency-only liveness —
already rejected by ADR-022 for the zombie case, and insufficient here for the
reuse case).

Refs: ADR-022 (OS-truth liveness — corrected by finding 3), ADR-024 §D2/§1
(register handshake, resident surfaces), PANTHEON_RULES.md A27/A26/A16/A21;
router CLAIM `20260602-024522`, ruling `20260602-023813`.
