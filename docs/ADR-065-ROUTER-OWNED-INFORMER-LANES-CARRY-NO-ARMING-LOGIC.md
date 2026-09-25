# ADR-065: Router-Owned Watcher/Informer — Lanes Carry No Arming Logic

## Status

**Proposed** — 2026-09-13. Owner decision, given directly in-session: *"two way with a watcher and informer logic in the router... that way lanes don't have to have arming logic; the watcher code is in the router."* Authored by `ra` (router / worker-plane, ADR-062/063). Requires SHA (`sirsi-hardware-admin`, who raised the A2A question) and SSA bind before implementation. Ledger: `rs-37-adr065-router-owned-informer` (blocked-by `rs-36`).

**Hardware-seat verdict 2026-09-16 (claude-io acting as sirsi-hardware-admin, owner delegation to 2026-09-19; item `20260916-040736`): ACCEPT as written, with two conditions on the build (rs-37): (1) node identity is config-pinned before or with the informer — rs-42 lands first or as one cut-over; (2) Decision 6 supervision covers the token holder's host binding (`token host == pinned node host`), not liveness alone. Cut-over needs rs-22-style proof-before-retire per lane per host, M1/M5 first. SSA verdict and owner bind still pending.

## Context

On 2026-09-13, one review cycle produced four independent failures with a single shape:

1. **Nine per-lane wake LaunchAgents on the M5** (`ai.sirsi.router.wake.<agent>`), each carrying its own `SIRSI_ROUTER_URL` — seven were wrong (direct HTTPS URL plus the plaintext host token, in a Codex sandbox with no DNS), so those lanes silently fell back to a local SQLite store. Fixed by hand (rs-34 correspondence); nothing prevents recurrence.
2. **`~/.zshenv` on the M5** sourced a fallback env file with the same wrong URL and token for any shell without `SIRSI_ROUTER_URL` already set. Fixed by hand; same recurrence exposure.
3. **`sirsi-software-admin` was 15 commits behind `main`** (frozen at `397eb638`, #732-era) because its lane has no network by design (rs-22e, PR #723) and the git-bundle handoff this fabric's canon names as *"the standing review hand-off"* had **never once been used for code** — `~/.sirsi/handoff/` held only markdown notes between other agents. Unblocked by a manually cut bundle (rs-36).
4. **A reply SHA sent was not visible in `ra`'s own `router pull`**, while the session-start health check reported *"SECOND thread registry is LIVE … two registries no code path reconciles."*

Every one of these is the same defect: **the logic that decides whether a lane is reachable — its arming loop, its store address, its transport config — lives in the lane.** Correctness is therefore the conjunction of N independently-maintained configurations, and it drifts. `internal/router/wake.go:912` `RunWakeLoop` is that arming logic: a per-lane process that polls `OpenItems` on a ticker and dispatches the lane's consumer, carrying all the hard-won safety logic (#636 progress gate, #642 PID adoption, hourly spawn ceiling, no-progress quarantine). Nine copies of it ran on the M5.

ADR-052 (accepted 2026-08-03) declares this fabric A2A-compliant and asserts *"Router SQLite store is the **sole** authority"* and an *"edge-triggered marker."* Under Rule A35 (Scope The Check To The Claim), failure 4 shows the sole-authority claim is not enforced, and the assessment in `docs/router-service/A2A_CONTRACT_ASSESSMENT.md` shows the seven-property table omits delivery/read acknowledgement entirely — the one property that would have made failures 3 and 4 visible instead of silent. The owner's verdict was blunt and, on this evidence, correct: *"you have an architecture which doesn't really work."*

Rule A29 is binding here: the wake subsystem (`WakePass`, `ProbeWakeReadiness`, `RunWakeLoop`, `wakemechanism.go`) **exists and must not be reimplemented** (Rule 0). This ADR does not rebuild it. It relocates the *subscriber* and removes the per-lane copies.

## Decision

**1. One informer per host subscribes for every registered agent; lanes run no wake loop.**
The store already provides the edge signal: `internal/routerstore/facade.go:82` fires `notifyWaiters(r.To)` on every fresh insert; `dispatch.go` exposes a cross-process per-agent FIFO (`ListenNotify` / `pokeFIFO`) and an edge-triggered `Wait` with a safety re-check. Today each lane's `RunWakeLoop` polls on a ticker instead. The informer is a **single process per host** that `ListenNotify`s on **all** registered agents and dispatches on the edge — replacing nine ticker loops with one subscriber. For networked lanes the service-side `Wait` (`remote.go:412`, already a server-side long-poll on the lane's behalf) is fanned out by a service-side informer the same way. The per-lane `ai.sirsi.router.wake.<agent>` LaunchAgents are retired; they *were* the arming logic.

**2. Two-way: the informer carries acknowledgement back.**
The gap found in `A2A_CONTRACT_ASSESSMENT.md` §6 — no delivery/read acknowledgement distinct from close — is closed by the same mechanism: a lane's receipt of a pushed item is recorded as `read_at` on the item by the informer, on the lane's behalf, before the lane's consumer runs. "SHA answered" becomes a state `ra`'s own pull can see. This is the eighth A2A property; ADR-052's table is amended to carry it.

**3. The router owns lane addressing; lanes hold no store address.**
`internal/router/registry.go:16` `AgentConfig` declares each lane's `Wake` mechanism and `Consumer` but **no delivery address** — today the spool location is an unstated convention (`~/.sirsi/relay/<agent>/`) that every lane must independently get right via `SIRSI_ROUTER_URL`. This ADR adds a declared `Delivery` to the registry (spool directory | endpoint | existing session-message adapter). The informer pushes **to** that address; the lane never configures **where the router is**. The entire `SIRSI_ROUTER_URL` misconfiguration class (failures 1 and 2) is removed by construction, not by vigilance.

**3a. Arming strategy is selected by the lane's declared agent type — the environment's vagaries live in one table, in the router.**
`AgentConfig.Type` (`claude`, `codex`, `gemini`, `qwen`, …) and `Wake.SessionMode` (`headless` | `interactive`) already exist in the registry. The informer derives its delivery strategy from them via a single type→strategy table rather than each lane encoding its own environment's quirks in its own config:

| Declared type / mode | Environment fact | Informer strategy |
|---|---|---|
| `codex` (workspace-write sandbox) | no DNS, filesystem-only reach (rs-22a/e) | filesystem push into the lane's spool dir; never HTTP |
| `claude`, `interactive` | existing session, must never be blind-spawned (A29 honest boundary) | `session-message` adapter; if no live session → **needs-owner**, stated not faked |
| `claude`, `headless` | process wake permitted | `cli-spawn` on the edge, under the moved #636/#642 gates |
| `gemma` / `qwen` (resident local model) | long-lived resident process, RAM-gated (A32/ADR-031) | notify the resident; never spawn a second; never start a cold model on a wake |

A lane may narrow but not widen its type's strategy (a `codex` lane cannot opt into HTTP). Adding an agent type is a row in this table plus a census matcher (A33) — the one place the fabric learns a new environment.

**3b. Connection is the lane's contract; arming is the router's.**
The division of labour is explicit. The **router** owns every decision about *how* to reach a lane (Decisions 1, 3, 3a). The **lane** owns exactly one obligation: **be registered truthfully (type, session mode, delivery address) and stay demonstrably connected so it does not miss work.** "Connected" is not a heartbeat — it is a *proven* state: the informer's push is acknowledged (Decision 2, `read_at`). A lane whose pushes go unacknowledged past a bound is `disconnected`, surfaced by `doctor`/`ctr` as **that lane's defect to fix** — re-register, reconnect, correct its declared type — never silently re-armed by the router guessing on its behalf. This is what makes a lane's liveness and its reachability two different facts (A27/A36: a heartbeat is necessary, never sufficient).

**4. Air-gapped lanes are reached by filesystem push; code sync becomes a delivered artifact.**
For a no-network lane (SSA, rs-22e), the informer's delivery is a write into the lane's spool directory — the relay already has that filesystem access. A git bundle is delivered through the same channel as a first-class artifact with the same acknowledgement, replacing the never-operated manual handoff (failure 3). This keeps rs-22e's security decision intact: no sandbox gains network.

**5. Safety logic moves intact.**
The #636 progress gate, #642 durable PID adoption, hourly spawn ceiling, and no-progress quarantine in `RunWakeLoop` move into the informer **as-is**, with their existing tests. Rule 0: relocated, not rewritten. Their invariants become fabric-wide (one ceiling per host) rather than per-lane.

**6. The informer is load-bearing and is supervised as such.**
It is registered in the thread census (A33) and recognised by pidfile as load-bearing (A32). An informer that dies must strand *visibly* — its absence is a `doctor`/`ctr` finding, never a silent stop.

## Alternatives Considered

1. **Keep per-lane loops; just fix the configs (what was done today).** Rejected — today *is* the evidence: seven of nine were wrong on one host, a fallback file was wrong on the same host, and a canon'd handoff was never operated. Correctness that must hold across N independent lanes does not hold.
2. **Adopt the literal Agent2Agent protocol wholesale as the fix.** Rejected as the *sole* fix, adopted in *semantics*. A2A is HTTP/JSON-RPC; it cannot reach a lane with no DNS, and giving sandboxes network reverses rs-22e. What this ADR takes from A2A is exactly its push-notification and task-lifecycle model — the server informs, clients do not poll, and delivery is acknowledged — carried over the spool for isolated lanes. That makes ADR-052's compliance claim true rather than aspirational.
3. **Grant network access to sandboxed lanes.** Rejected — reverses a deliberate least-privilege decision (owner, 2026-09-10) to solve a problem that filesystem push solves without it.
4. **A second dispatch authority (an out-of-band notifier beside the router).** Rejected — ADR-052 Rule 3 prohibits it, and it would reintroduce the registry split this ADR exists to end.

## Consequences

- **Positive**: one place to be correct; the `SIRSI_ROUTER_URL`/plist misconfiguration class is eliminated rather than patched; air-gapped lanes become reachable by push instead of stale by default; every delivery is acknowledged, so a silent non-arrival becomes an observable state; ADR-052's "sole authority" and "edge-triggered" claims become enforced properties.
- **Negative**: the informer is a new single point per host that must itself be supervised (Decision 6); registry gains a required `Delivery` field, so every existing `agents.json` row needs a migration entry; the per-lane LaunchAgent install path (`sirsi router wake-install`) is retired and its docs/tests removed.
- **Risk**: **cut-over while lanes are live.** Retiring a lane's wake plist before the informer is proven to reach it strands that lane. Mitigation is the same shape as the rs-22 relay cut-over: informer up → a per-lane push **receipt** verified (item delivered, `read_at` set) → *then* that lane's plist is booted out — one lane at a time, negative control (a lane whose push is deliberately misaddressed must surface as unreachable, not silently unarmed) before the first retirement. No whole-fleet flip.

## References

- `docs/ADR-052-A2A-CONDUIT-OPERATING-RULES.md` — the compliance claim this ADR makes enforceable; property table amended per Decision 2
- `docs/router-service/A2A_CONTRACT_ASSESSMENT.md` — property-by-property evidence; §6 is the ack gap; addendum records the live incident
- `docs/router-service/ROUTER_STACK_LAB_RECIPE.md` — component inventory; Change Log carries this ADR
- `docs/router-service/stacklab/WING.md` — `stacklab.wing.m1-ra`; this ADR is registered against the wing (task `ra-wing-router-v1`)
- PANTHEON_RULES.md — A29 (wake subsystem exists, do not rebuild), A32 (load-bearing recognition), A33 (census), A35 (scope the check to the claim), Rule 0
- Ledger: `rs-34-router-body-loss-empty-guard`, `rs-35-a2a-contract-assessment`, `rs-36-ssa-sandbox-sync-never-operated`, `rs-37-adr065-router-owned-informer`
- Router: `20260913-043215` (SHA's originating item), `20260913-063953` (assessment), `20260913-070742` (bundle notice)
- Code: `internal/router/wake.go:912` (`RunWakeLoop`), `internal/routerstore/dispatch.go` (`notifyWaiters`/`ListenNotify`/`Wait`), `internal/routerstore/remote.go:412` (`RemoteStore.Wait`), `internal/router/registry.go:16` (`AgentConfig`)
