# Sprint Plan — ADR-065 Router-Owned Informer, Phase 1 (Single Host)

**Sprint ID:** adr065-informer-phase1
**Workstream:** router-fabric (rs-37)
**ADR:** [ADR-065 — Router-Owned Watcher/Informer](../ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md)
**Owner agent:** ra (repo-scoped, sirsi-pantheon)
**Build authorization:** `sirsi-software-admin` (SSA), item `20261007-144957` — GRANTED 2026-10-07, bounded to the single-host informer + `AgentConfig.Delivery`, with corrections to Decisions 2 and 6 and this published sprint plan as a precondition. Separate, earlier **hardware-seat ACCEPT verdict** `20260916-040736` (`sirsi-hardware-admin`) carried two conditions ahead of SSA's gate: rs-42 node-identity pin — done, PR #765/#785 — and Decision 6 token-host==pinned-node-host, folded into Decision 6 below. (Corrected 2026-10-07: a prior draft of this header misattributed item `20261007-144957` to the hardware seat.)
**Product classification:** platform-foundation (router transport/dispatch internals; no end-user-facing surface changes).
**Phase 2 gate (named at that time, not pre-declared here):** retiring any lane's existing `ai.sirsi.router.wake.<agent>` LaunchAgent is a separate, independently reviewed per-lane sprint (the ADR's per-lane cut-over risk). Per A36 there is no standing owner gate on that work beyond security/privacy; if retirement raises a genuine security/privacy question, that gate is named explicitly, on its actual basis, at the Phase 2 sprint — this header does not pre-authorize or invent a general owner-approval requirement.

> Per Rule 17 and SSA's authorization condition 5: no retirement code in this sprint. Phase 1 builds and proves the informer + delivery declaration on one host, alongside the existing per-lane wake loops — it does not remove them.

## Commercialization Gate (portfolio law — required section)

- **Operator/User:** internal — every registered fabric lane (`ra`, `sirsi-software-admin`, `sirsi-hardware-admin`, claude-home/pantheon/io, codex lanes, resident models) and the owner-facing `doctor`/`ctr` surfaces that read lane reachability. No external end user.
- **Pain:** the 2026-09-13 incident class (ADR-065 Context, failures 1–4) — N independently-configured per-lane wake loops drift (wrong `SIRSI_ROUTER_URL`, a stale fallback env file, a 15-commit-stale air-gapped lane, a reply invisible to its own sender) because correctness is the conjunction of N configs instead of one.
- **Primary Workflow:** a work item lands → the single-host informer pushes it to the lane by its declared `Delivery`/`Type` strategy → the lane's own `router acknowledge` sets `read_at` → an unacknowledged or misaddressed push surfaces as `disconnected`/stranded on `doctor`/`ctr`, not as silence.
- **Trust Boundary:** read-only consumer of the ADR-067 adoption record (no new credential, no ADR-067 change); no sandboxed lane gains network (rs-22e stays intact — air-gapped delivery is Decision 4, explicitly deferred); the informer runs with the same host-local privilege `RunWakeLoop` already has today.
- **Support/Operational Owner:** `ra` (router-fabric workstream, rs-37); escalation path is the existing router decision-card flow (A32 owner reporting), not a new one.
- **Done Evidence (Phase 1):** `go build`/`go test -race` green including all five negative controls shown failing pre-fix and passing post-fix; coexistence proof (zero duplicate consumer spawn under a simultaneous edge); `doctor`/census registration of the informer as load-bearing; bounded manual observation window on one host (M1) with no double-dispatch and no regression to existing `RunWakeLoop` behavior; CHANGELOG + ADR-065 status entries; completion-proof scaffold validated (see "Completion Proof" below).
- **Willingness-to-pay / value field (SSA correction):** no external buyer — this is internal infrastructure, so "willingness-to-pay" is read as internal risk/cost reduction, recorded explicitly rather than left blank: the 2026-09-13 incident class cost a manual multi-hour cross-agent recovery (rs-34/rs-35/rs-36 correspondence) across two lanes for one misconfiguration; Phase 1's value is collapsing N independently-drifting configs to one, which converts that recurring recovery cost into a one-time build cost. No dollar figure attaches because the repo has no revenue instrumentation for internal tooling; the quantified unit is agent-hours of incident recovery avoided per recurrence, not currency.
- **Classification:** internal platform-foundation infrastructure; not independently commercialized. Matches the header above — recorded here to satisfy the portfolio-law section requirement, not because this sprint changes a buyer-facing surface.

## /goal

Phase 1 is complete when **all** of the following are true:

1. `AgentConfig` in `internal/router/registry.go` carries a `Delivery` field (spool-dir | endpoint | session-message), additive and optional — `agents.json` round-trips unchanged for every row that omits it (existing `extra`-preservation behavior, per the file's own round-trip contract).
2. A single-host informer process subscribes via the existing `ListenNotify`/`Wait` primitives (`internal/routerstore/dispatch.go`, `remote.go:412`) for **every** agent registered on that host, replacing N per-lane ticker polls with one subscriber — proven by running it alongside (not instead of) the existing `RunWakeLoop` lanes with no behavior regression.
3. Delivery strategy is selected by `AgentConfig.Type`/`Wake.SessionMode` via the one lookup table in ADR-065 Decision 3a (filesystem push for `codex`, session-message for interactive `claude`, cli-spawn for headless `claude`, resident-notify for `gemma`/`qwen`) — a lane may narrow but not widen its type's strategy, enforced by a test.
4. Delivery is distinct from read-acknowledgement (Decision 2, as corrected by SSA — see "Decision 2 correction" below): the informer's push sets a delivery-attempt marker only; the lane's own `router acknowledge` (the fenced channel already shipped for rs-34/rs-35) is the **only** writer of `read_at`. Neither adapter success nor a heartbeat may set `read_at`. Covered by Negative Control 5 below.
5. Decision 6 token-host validation: before pushing to a lane, the informer validates `token host == pinned node identity` via the ADR-067 adoption record's **live, non-revoked** state (never hostname shape, never client assertion, never a cached/prior adoption snapshot). Covered by Negative Controls 1 and 2 below.
6. **The coexistence dispatch boundary is one boundary, not two.** Both the informer and the existing `RunWakeLoop` call the *same* per-agent admission primitives already in `internal/router/wake.go` — `consumerPIDFilePath(routerRoot, agentID)`, `adoptRunningConsumer`, `dispatchConsumer`, `fabricDispatchQuarantined`, `fabricDispatchOverloaded`, `measurementWindowOpen`, `attendedSessionOwnsInbox` — keyed by the same per-lane PID file. Relocation (#636 progress gate, #642 PID adoption, hourly spawn ceiling, no-progress quarantine) means these functions move to be *shared* by both callers, not duplicated; neither path gets its own copy. **A shared PID filename alone is not atomic spawn admission (SSA correction):** the shared boundary must serialize check → adopt → spawn as one critical section at the admission primitives (e.g. the existing PID-file lock in `adoptRunningConsumer`), so two concurrent observers cannot both pass the check before either adopts/spawns. Covered by Negative Control 6 below.
7. The informer is recognized as load-bearing (A32) via pidfile, and registered in the thread census (A33) — an informer crash is a visible `doctor`/`ctr` finding, never a silent stop. Covered by Negative Control 3 below (informer absence).
8. **All five required negative controls** (table below) are new tests, run under `-race`, each shown failing with its respective fix reverted (A35: the check must be shown red before it is trusted green), each correlating the affected source item and receiving worker/lane in its assertion.

### The five required negative controls

| # | Control | Test name | Expected observable state | Source-item / receiving-worker correlation | Race | Red-before-green |
|---|---|---|---|---|---|---|
| 1 | **Mismatched token/node** | `TestInformerRefusesMismatchedTokenHost` | delivery refused; item stays undelivered (no delivery-attempt marker set); lane surfaces no push | synthetic item addressed to lane L; informer process runs under a token whose adopted host ≠ L's pinned node identity | `-race` | asserted failing with the Decision-6 check stubbed to always-pass |
| 2 | **Revoked credential despite prior adoption** | `TestInformerRefusesRevokedTokenAfterPriorAdoption` | delivery refused even though the token previously adopted L's machine-id (ADR-067 §3.2 exclusivity/revocation); informer re-checks revocation at push time, not at adoption time | item addressed to L; token revoked after its earlier successful adoption, before this push | `-race` | asserted failing if the informer caches the adoption check instead of re-validating per push |
| 3 | **Informer absence** | `TestDoctorSurfacesInformerAbsenceAsStranded` | with the informer process down, a lane's pending item is reported `stranded`/`unreachable` by `doctor`/`ctr` census (A32/A33), never silently retried as success | item addressed to L while informer pidfile is stale/missing | `-race` (runs in the race-enabled suite; the assertion is process-liveness, which `-race` cannot itself prove, but the test still executes under `-race` per SSA correction — no test is exempted from the enabled suite) | asserted failing if absence is reported as healthy or is unreported |
| 4 | **Misaddressed delivery** | `TestInformerSurfacesMisaddressedDeliveryAsUnreachable` | a `Delivery` address that does not resolve (bad spool path / dead endpoint / no live session for `session-message`) surfaces the lane as `unreachable`, never as a silent drop or a false delivery-attempt success | item addressed to L; L's registered `Delivery` target intentionally broken | `-race` | asserted failing if a failed push is swallowed without marking the lane unreachable |
| 5 | **Unacknowledged delivery (adapter success ≠ read-ack)** | `TestDeliveryAttemptDoesNotImplyReadAck` | simulated adapter success with no lane `router acknowledge` call must NOT read as acknowledged; `read_at` stays unset | item addressed to L; adapter reports success; L's consumer never calls `router acknowledge` | `-race` | asserted failing if adapter success is (mis)wired to set `read_at` |

Additionally, **Negative Control 6 (coexistence boundary, SSA point 2)** is required alongside the five above:

| # | Control | Test name | Expected observable state | Race | Red-before-green |
|---|---|---|---|---|---|
| 6 | **Simultaneous edge, zero duplicate spawn** | `TestCoexistenceSingleSpawnOnSimultaneousEdge` | the informer and `RunWakeLoop` both observe the same inbox edge for the same lane concurrently; exactly one `dispatchConsumer` call occurs; the second observer's `adoptRunningConsumer` call returns the first's PID instead of spawning a second process | `-race`, both observers started concurrently against the same store notification | asserted failing if the two paths use independent PID files/admission state instead of the shared one in /goal item 6 |

This closes the SSA gap directly: task-claim idempotency (the store refusing a second *claim*) does not by itself prove two wake observers cannot each *spawn a consumer process* before either claims — Negative Control 6 exercises the spawn path, not only the claim path.

### Decision 2 correction (supersedes ADR-065's original text for Phase 1)

ADR-065 Decision 2, as originally written, says the informer records `read_at` "on the lane's behalf" on push receipt. **SSA corrected this on 2026-10-07 (item `20261007-144957`): delivery-attempt and read-acknowledgement are distinct states.** The informer's push sets a delivery-attempt marker; only the lane's own `router acknowledge` call sets `read_at`. The ADR's Decision 2 prose is marked superseded-for-Phase-1 in the same commit as this sprint plan (see "ADR-065 status corrections" below) rather than left to silently contradict this plan.

**Explicitly NOT in Phase 1's /goal** (deferred to Phase 2, each its own gated sprint):
- Retiring any `ai.sirsi.router.wake.<agent>` LaunchAgent.
- The SSA air-gapped git-bundle-as-delivered-artifact path (Decision 4) — needs its own negative controls on the no-network sandbox and is independently risky.
- Multi-host fan-out (Phase 1 is one host, proven, before a second).

## Scope (Phase 1 only)

**In scope**
- `internal/router/registry.go`: add `Delivery` field + migration note in `agents.json` schema docs.
- New `internal/router/informer.go` (or equivalent): single-host subscriber loop, type→strategy table, progress/PID/spawn/quarantine logic **shared** (not copied) between `RunWakeLoop` and the informer via the existing functions named in /goal item 6.
- Delivery-attempt marker + read-ack distinction, built on the existing `router acknowledge` verb (already shipped, rs-34/rs-35) — no new ack primitive invented.
- Token-host validation against the ADR-067 adoption record, re-checked live (non-revoked state) at push time (read-only consumer of that record; this sprint does not touch ADR-067 itself).
- A33 census matcher + A32 load-bearing pidfile registration for the informer process.
- Tests: the six negative controls named above, plus the coexistence proof.

**Out of scope (deferred)**
- Any LaunchAgent retirement or `sirsi router wake-install` changes.
- Decision 4 (air-gapped filesystem delivery of code bundles).
- Multi-host informer fan-out.
- ADR-052's property-table amendment doc edit (tracked separately; this sprint only builds the property, not the doc amendment).

## Tasks (Phase 1, ordered)

| # | Task | Files touched | Type | Gate |
|---|------|---------------|------|------|
| 1 | This sprint plan | `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` | docs | done in this work item |
| 2 | Add `Delivery` field to `AgentConfig` + round-trip test | `internal/router/registry.go`, `registry_test.go` | code+test | CI green |
| 3 | Type→strategy lookup table + narrow-not-widen test | `internal/router/informer.go` (new) | code+test | CI green |
| 4 | Make progress gate / PID adoption / spawn ceiling / quarantine **shared functions** callable from both `wake.go` (`RunWakeLoop`) and the new informer — no duplication, existing tests moved and still passing | `internal/router/wake.go`, `internal/router/informer.go`, `wake_test.go` → shared test helpers | code+test | CI green, zero behavior diff on moved tests |
| 5 | Single-host subscriber loop over `ListenNotify`/`Wait`, calling the task-4 shared admission functions, run alongside existing `RunWakeLoop` (coexistence, not replacement); Negative Control 6 (simultaneous-edge, zero duplicate spawn) | `internal/router/informer.go` | code+test | CI green, Negative Control 6 shown failing pre-fix |
| 6 | Delivery-attempt marker distinct from `read_at` (Decision 2 correction); Negative Control 5 | `internal/router/informer.go`, `internal/routerstore/*` (read-only use of existing ack verb) | code+test | CI green, Negative Control 5 shown failing pre-fix |
| 7 | Token-host validation against the live ADR-067 adoption record; Negative Controls 1 and 2 (mismatch; revoked-despite-prior-adoption) | `internal/router/informer.go` | code+test | CI green, Negative Controls 1 and 2 shown failing pre-fix |
| 8 | A33 census matcher row + A32 load-bearing pidfile for the informer; Negative Controls 3 and 4 (informer absence; misaddressed delivery surface as unreachable, not success) | `internal/router/census.go` (or equivalent), `internal/guard/loadbearing.go` | code+test | CI green, Negative Controls 3 and 4 shown failing pre-fix |
| 9 | CHANGELOG + ADR-065 status note (authorization, attribution correction, Decision 2 supersession note, rollback contract reference) | `CHANGELOG.md`, `docs/ADR-065-...md` | docs | Phase-1 commit |

No task in this list retires a live lane. Task 5's "coexistence" requirement is deliberate: Phase 1 proves the informer works beside the existing arming logic — sharing its admission state, not parallel to it — before anything is asked to depend on it exclusively.

## Tests / verification

- `go build ./cmd/sirsi/` succeeds.
- `go test ./internal/router/... ./internal/routerstore/... -race` green, including all six negative controls (five required controls + the coexistence control) shown failing with each respective fix reverted (A35).
- Existing `RunWakeLoop`/wake tests continue to pass unmodified — proves Phase 1 added a parallel observer sharing admission state, rather than silently changing the live one.
- `golangci-lint run ./...` clean.
- Manual verification on one host (M1): informer and the existing per-lane `RunWakeLoop` loops run side by side for a bounded observation window with no double-dispatch of the same item, observed via the shared PID-file admission state (not merely assumed from store claim semantics — see Negative Control 6).
  - **Bounded observation parameters (SSA correction — named before execution, not left implicit):** duration 60 minutes wall-clock on M1; coverage is every agent registered on that host at observation start (current M1 registry, not a sampled subset); start criterion is both the informer and all pre-existing `RunWakeLoop` lanes reporting live via `doctor`; stop criterion is either the 60-minute bound or 20 observed dispatch edges, whichever comes first. Receipts: each dispatch's `dispatchConsumer` call site increments a counter already exposed by the shared admission primitives (task 4), read via `sirsi router doctor --json` before/after; the zero-duplicate threshold is exactly 0 — any edge with more than one `dispatchConsumer` call for the same lane in the same edge window is a fail, not a tolerance band. No-regression threshold: 0 existing wake test failures and 0 new `doctor` stranded findings versus the pre-observation baseline.

## Rollback contract (authorization condition 5)

Phase 1 adds a process; it does not retire one, so rollback is "stop the new thing," not "restore the old thing" — but the contract below is explicit rather than assumed:

1. **Stop:** `sirsi router informer stop` (or kill the pidfile'd process) terminates the informer. No other process depends on its liveness — `RunWakeLoop` lanes were never modified to require it.
2. **Verify existing wake paths still function:** after the informer is stopped, confirm each lane's `RunWakeLoop` continues to dispatch on its own ticker exactly as it did before Phase 1 shipped (the existing wake test suite, re-run with the informer absent, is the check).
3. **Preserve queued work and read-ack truth:** the informer never owns data the store doesn't already own — delivery-attempt markers and `read_at` live in the routerstore, not in informer-local state, so stopping the informer loses no queued item and no acknowledgement history. Evidence: a `router pull`/`router show` diff before and after stop shows no item state regression.
4. **Verify recovery:** restarting the informer resumes pushing to lanes whose items are still pending (the store, not the informer, is the source of truth for "pending"); no manual reconciliation step is required. Evidence: a synthetic item sent while the informer is down is delivered once the informer restarts, without a duplicate dispatch from the `RunWakeLoop` lane that was covering it meanwhile.

This is bounded observation, not a formal DR drill: Phase 1 runs on one host (M1) alongside the existing lanes, so "rollback" in Phase 1 is operationally cheap by construction (A36: no code removed, no lane retired). Phase 2's per-lane retirement sprint needs a heavier rollback contract (plist reinstall) and will restate one.

## Completion Proof (SSA correction — was missing)

sirsi-pantheon did not carry `.agents/completion.contract.json` before this correction; it is added in this same commit (`.agents/completion.contract.json`, `canon_documents` pointing at `PANTHEON_RULES.md`, `docs/COMMERCIALIZATION_GATE.md`, `docs/ARCHITECTURE_DESIGN.md`, `docs/SAFETY_DESIGN.md`, the portfolio manifesto; `required_verification_commands` for `go build`, `go vet`, `golangci-lint`, `go test ./... -race`). This does not replace the gates named in "Tests / verification" above — it wraps them in the portfolio's proof format alongside the focused race checks, per authorization condition 5.

- **Proof path:** `.agents/proofs/rs-37-adr065-informer-phase1.json`.
- **Init:** `python3 ${DEVELOPMENT_ROOT:-$HOME/Development}/tools/agent_completion_gate.py init-proof --repo . --work-item rs-37-adr065-informer-phase1 --agent-id ra`.
- **Validate before ready-for-review/complete:** `python3 ${DEVELOPMENT_ROOT:-$HOME/Development}/tools/agent_completion_gate.py validate --repo . --proof .agents/proofs/rs-37-adr065-informer-phase1.json`.
- **Close:** `sirsi router close <router-id> --proof .agents/proofs/rs-37-adr065-informer-phase1.json --result @path/to/result.md`.
- **Requirement trace (filled progressively as tasks 2–9 land):** each `done_definition` category in the contract maps to a task above — `technical` → tasks 2–8 + the six negative controls; `operational` → task 9 (CHANGELOG/ADR note) + the rollback contract; `narrative` → the three-home publication status below; `design` → this sprint plan and the ADR-065 Decision 2/6 corrections; `product` → `/goal` items 1–8.
- **Working status (as of this correction, 2026-10-07):** the contract scaffold exists; the proof file does not yet exist (no implementation task has started). The gate tool path named in portfolio canon, `/Users/thekryptodragon/Development/tools/agent_completion_gate.py`, is a `thekryptodragon`-machine path and is **not present on this host** (`sirsimasterdev`'s M) under any `$DEVELOPMENT_ROOT` resolution checked here — this is an **actual blocker**, recorded rather than assumed away: the init-proof/validate commands above cannot run from this worktree until the tool is reachable (either synced to this host under `$DEVELOPMENT_ROOT`, or run from a session on the host where it exists). **Recovery action:** before task 2 begins, locate or sync the tool for this host, or hand the `init-proof`/`validate` steps to a session running on the host that has it; do not mark Phase 1 complete without a validated proof, and do not claim the proof ran if the tool was unreachable.

## Three-Home Publication Status (SSA correction — was missing)

Per the portfolio's three-home publication rule (`AGENTS.md` § "Three-home publication rule"), this sprint plan and ADR-065 are **not fully published** merely by existing in this PR — the rule requires (1) the authoritative repo source, (2) a Desktop Owner Reading Room entry, (3) a native Workspace copy, each naming the others.

- **(1) Repository source:** `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` + `docs/ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md`, PR #1034. **Not yet on `origin/main`** — the PR is open, under re-review, not merged. Per A37 (A Record Exists Only On Origin), this document does not count as published canon until it lands on `origin/main`; this status line itself is the honest record of that, not a claim of completion.
- **(2) Desktop Owner Reading Room:** stable identifier reserved — `Sirsi - Owner Reading Room/Pantheon/ADR-065-router-informer.md` (to be created on merge). **Blocked on this host:** `~/Desktop/Sirsi - Owner Reading Room` on `sirsimasterdev`'s machine is a symlink to `/Users/thekryptodragon/Desktop/Sirsi/00 Owner Reading Room`, which **does not exist on this host** (verified: `ls` on the target returns "No such file or directory"). This is a cross-machine path mismatch, not a sync lag — the Reading Room lives on the owner's other machine. **Recovery action:** the entry must be created from a session running on the host where that path resolves, after PR #1034 merges; this repo record and the stable identifier above are left so that session updates the existing entry rather than creating a silent duplicate.
- **(3) Native Workspace copy:** not yet created — blocked on (1) (nothing to mirror until the PR merges) and likely blocked on the same host-access question as (2). Recorded as **blocked**, not silently assumed synchronized.

This status is current as of 2026-10-07 and must be updated (not re-written from scratch) once PR #1034 merges and once each mirror is actually created — a later session should edit the three bullets above in place, not add a second status block.

## Risks

- **Double-delivery / double-spawn risk:** running the informer alongside existing per-lane loops means the same edge could in principle be observed by both. Mitigation: /goal item 6 makes both paths share the same per-agent PID-file admission state (not independent copies), and Negative Control 6 asserts zero duplicate spawn directly, rather than inferring it from the store's claim/lease semantics alone (store claim idempotency proves a second *claim* is a no-op; it does not by itself prove a second *process spawn* never happens before either side claims).
- **Token-host validation risk:** a bug here that is too strict strands a legitimate lane; a bug that is too loose reopens the cross-host impersonation class PR #765 deliberately closed, or the revocation-bypass class Negative Control 2 targets. Mitigation: Negative Controls 1 and 2, plus reuse of the existing `TestHostTokenAuthorizesOnlyItsOwnHost`/`TestThreadAuthorityIsHostScoped` suite as a regression fence (do not weaken those tests to make this pass).
- **Silent-failure risk (absence/misaddress):** a push that fails must not look like success. Mitigation: Negative Controls 3 and 4 require `doctor`/`ctr` to surface both informer absence and misaddressed delivery as stranded/unreachable states, matching the ADR's "disconnected is the lane's defect to fix, never silently re-armed" stance (ADR-065 Decision 3b).
- **Scope creep into retirement:** the ADR's own alternatives-considered section and SSA's authorization are explicit that cut-over is lane-by-lane with proof first. Mitigation: Phase 2 is a separate sprint plan, not a continuation task inside this one, and this header's Phase 2 gate language does not pre-invent an owner-approval requirement beyond security/privacy (A36).

## Confidence declarations (Rule A23)

- **Confidence this is the right next action:** High — SSA's authorization item (`20261007-144957`) names exactly these conditions, and ledger note on `rs-37` states "next action is the Rule-17 sprint plan ... not yet published."
- **Confidence in the task breakdown matching the ADR's decisions 1/2/3/3a/5/6:** High — each task cites the ADR decision number and the existing code it relocates or shares; nothing here reimplements wake/dispatch primitives (Rule 0 compliance checked against A29).
- **Confidence in the token-host validation design (task 7) and the revocation-recheck requirement (Negative Control 2):** Medium — depends on reading the ADR-067 adoption record's actual shape (confirmed: `host_tokens.machine_id`, unique among non-revoked tokens, §3.2) before implementation; flagged as the task needing a short read-first pass, not a blind diff.
- **Confidence in the coexistence boundary design (task 4/5, Negative Control 6):** Medium — the specific shared functions are named from reading `internal/router/wake.go` directly (`consumerPIDFilePath`, `adoptRunningConsumer`, `dispatchConsumer`, `fabricDispatchQuarantined`, `fabricDispatchOverloaded`, `measurementWindowOpen`, `attendedSessionOwnsInbox`), but the exact refactor shape (shared package-level functions vs. an interface) is an implementation-time decision, not fixed by this plan.

## ETA / check-back

- This plan is published; implementation (tasks 2–9) begins in a subsequent work session under the same `rs-37` ledger entry.
- Phase 1 does not require further owner sign-off beyond SSA's existing authorization (A36: pre-approved work proceeds without a new gate) — it is not "new scope," it is the condition SSA already named as a precondition to the build.
- On Phase 1 completion, a Phase 2 sprint plan is written separately for the first per-lane retirement (M1, then M5, per the ADR's one-lane-at-a-time risk mitigation), naming its own rollback contract and any genuine security/privacy gate on its own basis.
