# Design: Progress-Gated Dispatch — stop the wake-loop from burning metered tokens

**Status:** proposed · **Issue:** #636 · **Author:** claude-home · **Date:** 2026-08-07
**Implementable by:** claude-home or gemma. Every change below names a file, a function, and a test.

---

## 1. The incident, measured

One lane (`claude-finalwishes`) on the owner's machine, 2026-08-06:

| Metric | Value |
|---|---|
| Headless sessions spawned in one day | **1,197** (vs 38 the day before) |
| `dispatched consumer` events in the wake log | **1,221** |
| Transcripts / bytes in 24h | 1,228 / **98.8 MB** |
| Sessions that achieved **nothing** (6–30 lines, median 17) | **1,223 of 1,237 — 99%** |
| Sessions that did real work (>30 lines) | **12 — 1%** |
| Cache-read tokens | **59.2 M** |
| Cache-write tokens | 2.3 M |
| Output tokens | 484 K |

**99% of spawns paid a full agent cold-start — roughly 48,000 tokens of cached context
(CLAUDE.md + memory + skills) — to load, glance at the inbox, and exit.** The cost was not
runaway *work*; it was the fixed startup price of an agent, paid 1,197 times.

The owner discovered this from a 1.0 GB process in a process list and an "approaching weekly
usage limit" banner. **No Pantheon surface reported it.**

---

## 2. Root cause

`internal/router/wake.go`, `RunWakeLoop`, the dispatch gate:

```go
if consumer != nil && !consumer.Resident && lerr == nil && depth > 0 && !run.running() {
    next, derr := dispatchConsumer(consumer)
    ...
}
```

The gate is **thoughtfully designed and still wrong**. Its own comment anticipates fork-storms
and rejects a timer in favour of consumer liveness: *"dispatch only when nothing is in flight. A
slow-but-healthy agent holds the slot for as long as it genuinely needs, and a dead one frees it
the instant it exits."*

That reasoning covers two consumer outcomes:

1. works long, drains the inbox → holds the slot; and
2. dies immediately and visibly → frees the slot.

**It does not cover the third: exits quickly having made no progress.** In that case `depth > 0`
stays true forever (nothing drained) and `!run.running()` becomes true every tick (the consumer
exited). The edge-trigger degenerates into a level-trigger — precisely the failure the comment
set out to prevent. Observed: ~8 dispatches per loop incarnation, one roughly every tick.

**There is no progress requirement anywhere in the loop.** A consumer that drains five items and
one that drains zero are indistinguishable to the gate.

Two compounding factors:

- **The guard is process-local.** `run` is a struct field. The LaunchAgent is `KeepAlive=true`,
  so every loop restart resets it to "nothing in flight." 151 restarts were logged.
- **`launchctl disable` does not hold** on these lanes — it reads back `false` and the lane is
  bootstrapped again. Only renaming the plist stops it. Any remediation built on `disable` is a
  no-op. (`router quarantine-worker` already knows this and renames; the wake lanes do not.)

### 2.1 Why the existing guard did not catch it

`internal/guard/runaway.go::checkRunawayExecutor` counts **concurrent** headless sessions
(`countHeadlessAgentSessions`). The damage here was **cumulative**: 1,197 short-lived sessions
never produce a high concurrent count. A concurrency gauge is structurally blind to this failure
mode.

Worse, `router quarantine-worker` — the advertised remediation for the runaway finding — states
in both its help text and its source comment: *"Wake-loop watchers and the router supervisor are
never touched — the incident's watchers were healthy."* That assumption held for the 2026-07-04
incident. It is false here: **the wake-loop watcher IS the spawner.** Running the remediation
left the runaway fully intact while reporting success.

---

## 3. Design

Four changes, ordered by value. **C1 alone stops the bleeding**; the rest make it
undetectable-by-accident rather than merely fixed.

### C1 — Progress-gated dispatch with backoff *(the fix)*

**File:** `internal/router/wake.go`

Record inbox depth at dispatch; on consumer exit, require it to have decreased. If it did not,
back off exponentially instead of re-dispatching at the next tick.

```go
// consumerRun gains progress accounting.
type consumerRun struct {
    done      chan struct{}
    pid       int
    err       error
    depthAtDispatch int       // NEW: inbox depth when this consumer started
    startedAt       time.Time // NEW
}

// wakeLoopBackoff returns the delay before a lane may dispatch again after
// `fruitless` consecutive no-progress exits. Bounded so a genuinely stuck lane
// costs O(log n) spawns per day rather than O(ticks).
func wakeLoopBackoff(fruitless int) time.Duration
```

Gate becomes:

```go
if consumer != nil && !consumer.Resident && lerr == nil && depth > 0 &&
   !run.running() && now.After(nextDispatchAllowed) {
```

On consumer exit:

```go
if run.depthAtDispatch >= 0 && depth >= run.depthAtDispatch {
    fruitless++
    nextDispatchAllowed = now.Add(wakeLoopBackoff(fruitless))
    log.Printf("wake-loop %s: consumer made NO PROGRESS (depth %d -> %d); "+
        "backing off %s (%d consecutive)", agentID, run.depthAtDispatch, depth,
        wakeLoopBackoff(fruitless), fruitless)
} else {
    fruitless = 0
}
```

Backoff schedule: `1×, 2×, 4×, 8×… interval`, capped at **1 hour**. After
`wakeLoopFruitlessQuarantine` (**10**) consecutive fruitless dispatches, stop dispatching
entirely, set the thread status to `blocked` with `last_error` naming the lane, and let the
supervisor surface it. **A lane that cannot make progress is a bug to report, not a spawn to
retry.**

> Depth is the cheapest available progress proxy and is already read every tick. It is not
> perfect — an agent may legitimately work an item without closing it within one dispatch. That
> is why the response is *backoff*, not an immediate block: a slow-but-real agent pays one
> doubled interval, while a no-op lane decays to one spawn/hour and then stops.

**Tests** (`internal/router/wake_progress_test.go`):

- `TestDispatchBacksOffWhenDepthUnchanged` — depth 19 → 19 ⇒ next dispatch deferred.
- `TestDispatchResetsBackoffOnProgress` — depth 19 → 17 ⇒ `fruitless == 0`, no deferral.
- `TestBackoffIsBoundedAndMonotonic` — strictly increasing, never exceeds 1h.
- `TestFruitlessLaneQuarantinesNotSpins` — 10 fruitless exits ⇒ dispatch stops, status `blocked`.
- `TestSlowConsumerIsNotPenalised` — long-running consumer that drains ⇒ never backs off.
- **Negative control (required):** revert C1 and confirm
  `TestDispatchBacksOffWhenDepthUnchanged` fails. A test that passes without the fix covers
  nothing.

### C2 — Durable dispatch lease *(survives loop restart)*

**File:** `internal/routerstore/dispatch.go`

`run` is in-process; `KeepAlive=true` means restarts reset it. Persist the lease so a restarted
loop sees an in-flight consumer.

```go
type DispatchLease struct {
    Agent           string
    PID             int
    StartedAt       time.Time
    ExpiresAt       time.Time
    DepthAtDispatch int
    IdempotencyKey  string // agent + depth + minute bucket
}

// AcquireDispatchLease returns (lease, true) only if no live, unexpired lease
// exists for the agent. Single transaction — two loops racing cannot both win.
func (s *Store) AcquireDispatchLease(agent string, ttl time.Duration, depth int) (*DispatchLease, bool, error)
func (s *Store) ReleaseDispatchLease(agent string, pid int) error
func (s *Store) ActiveDispatchLease(agent string) (*DispatchLease, bool, error)
```

A lease whose PID is dead is treated as expired (reuse the existing liveness probe — do **not**
trust `ExpiresAt` alone; a killed consumer must free the slot immediately, preserving the
current design's best property).

This matches the invariant `dispatch.go` already documents: *"claim is the only door to
execution; Wait observes, it never claims."*

**Tests:** concurrent `AcquireDispatchLease` ⇒ exactly one winner; dead-PID lease is reacquirable;
lease survives a simulated loop restart and blocks a second dispatch.

### C3 — Hard spawn-rate ceiling *(defence in depth)*

**File:** `internal/routerstore/dispatch.go` + enforced in `wake.go`

Independent of any gate logic, cap dispatches per agent per rolling hour
(`wakeLoopMaxSpawnsPerHour`, default **12**). On breach: refuse, log, mark the lane `blocked`.

This is the backstop that would have capped this incident at ~288 sessions/day instead of 1,197
**even with C1 and C2 both broken.** Ceilings that depend on correct logic elsewhere are not
ceilings.

**Test:** 13 dispatches in one simulated hour ⇒ 13th refused, lane blocked.

### C4 — Make the detector measure the right quantity

**File:** `internal/guard/runaway.go`

1. Add `countSpawnRatePerLane()` — sessions started per lane per hour, from the wake logs or the
   new lease table. Warn at 12/h, critical at 30/h. **Keep the concurrency check**; it catches a
   different shape. Report both.
2. Correct `router quarantine-worker`: extend it to wake lanes, or amend its help text and source
   comment which currently assert wake loops are never the cause. **A remediation that is wrong
   about its own scope is worse than none — it reports success while the leak runs.**
3. Surface **cost, not just processes**: sessions spawned and tokens attributed per lane, so a
   runaway is visible as spend. The owner's only signal was a billing banner.

**Test:** a fixture of 100 sessions each lasting 20s across one hour ⇒ concurrency check stays
green, rate check goes critical. This is the exact 2026-08-06 shape and must not pass silently.

---

## 4. Order of work

| Step | Change | Effect |
|---|---|---|
| 1 | C1 progress gate + backoff | Stops the bleeding. ~1,200 spawns/day → ≤24 |
| 2 | C3 rate ceiling | Bounds any future variant, logic-independent |
| 3 | C2 durable lease | Closes the restart hole; satisfies ADR-057 |
| 4 | C4 detector + remediation truth | Makes it impossible to miss next time |

C1 and C3 are each a few dozen lines and independently shippable. **Do not ship C4 first** — a
better detector on an unfixed leak just watches money burn more precisely.

---

## 5. Re-arm criteria

The wake lanes are parked (`~/Library/LaunchAgents/*.OFF-owner-20260807`) by owner directive:
*"the only consistent loops should be you and router/ledger."* Restore **one** lane only after:

1. C1 + C3 merged with the negative control passing;
2. a 24h soak on one lane showing **≤ 24 spawns/day** and non-decreasing progress; and
3. `sirsi diagnose` reporting spawn-rate per lane.

⚠ **All 24 agent lanes have a `consumer.command` and therefore spawn** (measured 2026-08-07 from
`.agents/idea-router/agents.json`; only `gemma-pantheon`, `horus`, `owner`, `user` are
consumer-less). Any note claiming most lanes are "watch-only" is stale — every lane is affected.

---

## 6. Product opportunity

This is a category problem, not a local bug. Any self-hosted runtime that spawns **metered**
agent sessions on a loop has this exposure, and a *merely backlogged* lane is enough to trigger
it — no bug, no bad actor, no malice required. The failure is silent by construction: processes
are short-lived, concurrency stays low, disk barely moves, and the only signal is a bill.

Three controls Pantheon can own as first-class features rather than diagnostics:

- **Spend as a supervised resource** — per-lane token budgets with enforcement, not just
  observation. "This lane may spend N tokens/day" is the control users actually want.
- **Progress as a dispatch precondition** — work that does not reduce a queue is not work.
  Generalises well beyond this loop.
- **Cumulative-rate alarms alongside concurrency gauges** — the 2026-07-04 incident was a
  concurrency spike, this one was a rate spike. Both are runaways; only one was instrumented.

**The general lesson:** the guard here was written by someone who had already thought about
fork-storms and reasoned carefully about the right instrument. It still failed, because the
unhandled case was not "spawn too fast" but "finish too cheaply." Cost-bearing loops need a
progress invariant, not just a liveness one.
