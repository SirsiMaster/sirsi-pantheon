# Sprint Plan — ADR-065 Router-Owned Informer, Phase 1 (Single Host)

**Sprint ID:** adr065-informer-phase1
**Workstream:** router-fabric (rs-37)
**ADR:** [ADR-065 — Router-Owned Watcher/Informer](../ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md)
**Owner agent:** ra (repo-scoped, sirsi-pantheon)
**Build authorization:** `sirsi-hardware-admin`, item `20261007-144957` (hardware-seat ACCEPT verdict `20260916-040736` carried two prior conditions: rs-42 node-identity pin — done, PR #765/#785 — and Decision 6 token-host==pinned-node-host, folded into Decision 6 below).
**Product classification:** platform-foundation (router transport/dispatch internals; no end-user-facing surface changes).
**User authorization required before:** retiring any lane's existing `ai.sirsi.router.wake.<agent>` LaunchAgent (that is Phase 2, gated separately per the ADR's per-lane cut-over risk).

> Per Rule 17 and SSA's authorization condition 5: no retirement code in this sprint. Phase 1 builds and proves the informer + delivery declaration on one host, alongside the existing per-lane wake loops — it does not remove them.

## /goal

Phase 1 is complete when **all** of the following are true:

1. `AgentConfig` in `internal/router/registry.go` carries a `Delivery` field (spool-dir | endpoint | session-message), additive and optional — `agents.json` round-trips unchanged for every row that omits it (existing `extra`-preservation behavior, per the file's own round-trip contract).
2. A single-host informer process subscribes via the existing `ListenNotify`/`Wait` primitives (`internal/routerstore/dispatch.go`, `remote.go:412`) for **every** agent registered on that host, replacing N per-lane ticker polls with one subscriber — proven by running it alongside (not instead of) the existing `RunWakeLoop` lanes with no behavior regression.
3. Delivery strategy is selected by `AgentConfig.Type`/`Wake.SessionMode` via the one lookup table in ADR-065 Decision 3a (filesystem push for `codex`, session-message for interactive `claude`, cli-spawn for headless `claude`, resident-notify for `gemma`/`qwen`) — a lane may narrow but not widen its type's strategy, enforced by a test.
4. Delivery is distinct from read-acknowledgement (Decision 2): the informer's push sets a delivery-attempt marker; the lane's own `router acknowledge` (the fenced channel already shipped for rs-34/rs-35) sets `read_at`. Neither adapter success nor a heartbeat may set `read_at` — enforced by a negative-control test (simulated adapter success with no lane acknowledge must NOT read as acknowledged).
5. Decision 6 token-host validation: before pushing to a lane, the informer validates `token host == pinned node identity` via the ADR-067 adoption record (never hostname shape or client assertion) — enforced by a negative control (mismatched token/node must refuse delivery, not silently push).
6. The #636 progress gate, #642 PID adoption, hourly spawn ceiling, and no-progress quarantine move into the informer **intact, with their existing tests** (Rule 0 — relocated, not rewritten); the existing `RunWakeLoop` tests continue to pass unmodified (both code paths coexist in Phase 1).
7. The informer is recognized as load-bearing (A32) via pidfile, and registered in the thread census (A33) — an informer crash is a visible `doctor`/`ctr` finding, never a silent stop.
8. Negative controls for the five properties above are new tests, run under `-race`, and are shown failing with each respective fix reverted (A35: the check must be shown red before it is trusted green).

**Explicitly NOT in Phase 1's /goal** (deferred to Phase 2, each its own gated sprint):
- Retiring any `ai.sirsi.router.wake.<agent>` LaunchAgent.
- The SSA air-gapped git-bundle-as-delivered-artifact path (Decision 4) — needs its own negative controls on the no-network sandbox and is independently risky.
- Multi-host fan-out (Phase 1 is one host, proven, before a second).

## Scope (Phase 1 only)

**In scope**
- `internal/router/registry.go`: add `Delivery` field + migration note in `agents.json` schema docs.
- New `internal/router/informer.go` (or equivalent): single-host subscriber loop, type→strategy table, progress/PID/spawn/quarantine logic relocated from `wake.go`.
- Delivery-attempt marker + read-ack distinction, built on the existing `router acknowledge` verb (already shipped, rs-34/rs-35) — no new ack primitive invented.
- Token-host validation against the ADR-067 adoption record (read-only consumer of that record; this sprint does not touch ADR-067 itself).
- A33 census matcher + A32 load-bearing pidfile registration for the informer process.
- Tests: 5 negative controls named in /goal item 8, plus coexistence tests proving the old per-lane loops are unaffected.

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
| 4 | Relocate progress gate / PID adoption / spawn ceiling / quarantine from `wake.go` into the informer, existing tests moved and still passing | `internal/router/informer.go`, `wake_test.go` → new test file | code+test | CI green, zero behavior diff on moved tests |
| 5 | Single-host subscriber loop over `ListenNotify`/`Wait`, run alongside existing `RunWakeLoop` (coexistence, not replacement) | `internal/router/informer.go` | code+test | CI green |
| 6 | Delivery-attempt marker distinct from `read_at`; negative control (adapter success without lane ack ≠ acknowledged) | `internal/router/informer.go`, `internal/routerstore/*` (read-only use of existing ack verb) | code+test | CI green, negative control shown failing pre-fix |
| 7 | Token-host validation against ADR-067 adoption record; negative control (mismatched token/node refused) | `internal/router/informer.go` | code+test | CI green, negative control shown failing pre-fix |
| 8 | A33 census matcher row + A32 load-bearing pidfile for the informer | `internal/router/census.go` (or equivalent), `internal/guard/loadbearing.go` | code+test | CI green |
| 9 | CHANGELOG + ADR-065 status note (authorization + Phase 1 scope recorded) | `CHANGELOG.md`, `docs/ADR-065-...md` | docs | Phase-1 commit |

No task in this list retires a live lane. Task 5's "coexistence" requirement is deliberate: Phase 1 proves the informer works beside the existing arming logic before anything is asked to depend on it exclusively.

## Tests / verification

- `go build ./cmd/sirsi/` succeeds.
- `go test ./internal/router/... ./internal/routerstore/... -race` green, including the 5 new negative controls shown failing with each respective fix reverted (A35).
- Existing `RunWakeLoop`/wake tests continue to pass unmodified — proves Phase 1 added a parallel path rather than silently changing the live one.
- `golangci-lint run ./...` clean.
- Manual verification on one host (M1): informer and the existing per-lane `RunWakeLoop` loops run side by side for a bounded observation window with no double-dispatch of the same item (idempotency already guaranteed by the store's claim semantics, but observed not just assumed).

## Risks

- **Double-delivery risk:** running the informer alongside existing per-lane loops means the same edge could in principle be observed by both. Mitigation: the store's existing claim/lease semantics (already load-bearing, untouched by this sprint) make a second claim attempt a no-op; task 5's coexistence test asserts this explicitly rather than assuming it.
- **Token-host validation risk:** a bug here that is too strict strands a legitimate lane; a bug that is too loose reopens the cross-host impersonation class PR #765 deliberately closed. Mitigation: task 7's negative control plus reuse of the existing `TestHostTokenAuthorizesOnlyItsOwnHost`/`TestThreadAuthorityIsHostScoped` suite as a regression fence (do not weaken those tests to make this pass).
- **Scope creep into retirement:** the ADR's own alternatives-considered section and SSA's authorization are explicit that cut-over is lane-by-lane with proof first. Mitigation: Phase 2 is a separate sprint plan, not a continuation task inside this one.

## Confidence declarations (Rule A23)

- **Confidence this is the right next action:** High — SSA's authorization item (`20261007-144957`) names exactly these 5 conditions, and ledger note on `rs-37` states "next action is the Rule-17 sprint plan ... not yet published."
- **Confidence in the task breakdown matching the ADR's decisions 1/2/3/3a/5/6:** High — each task cites the ADR decision number and the existing code it relocates or extends; nothing here reimplements wake/dispatch primitives (Rule 0 compliance checked against A29).
- **Confidence in the token-host validation design (task 7):** Medium — depends on reading the ADR-067 adoption record's actual shape before implementation; flagged as the one task needing a short read-first pass, not a blind diff.

## ETA / check-back

- This plan is published; implementation (tasks 2–9) begins in a subsequent work session under the same `rs-37` ledger entry.
- Phase 1 does not require further owner sign-off beyond SSA's existing authorization (A36: pre-approved work proceeds without a new gate) — it is not "new scope," it is the condition SSA already named as a precondition to the build.
- On Phase 1 completion, a Phase 2 sprint plan is written separately for the first per-lane retirement (M1, then M5, per the ADR's one-lane-at-a-time risk mitigation).
