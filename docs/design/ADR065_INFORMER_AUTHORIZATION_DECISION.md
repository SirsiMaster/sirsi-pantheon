# Decision note: where ADR-065 informer authorization (NC1/NC2) is enforced

**Status:** option (a') **adopted by Ra 2026-11-28** for the service-side predicate (see §9); part A shipped as #1463 (default off); part B pending · **Task:** `rs-37-adr065-router-owned-informer` · **Author:** ra · **Date:** 2026-11-24
**This note does not complete rs-37.** rs-37 stays open: on `origin/main` only Phase-1 task 2 (`AgentConfig.Delivery`) exists; no informer runs anywhere; no wake LaunchAgent is retired.
**Closes:** ADR-065 acceptance condition 4 (the open design question) once adopted. Adoption is Ra's design call, but any variant that changes who may call what is a security-relevant change and keeps the security gate (SSA review; owner gate if a credential surface is added).

Inputs read: `docs/ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md`, `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md`, and `origin/ra/rs37b-informer-host-wiring` (8 commits ahead of `main`, 1,561 insertions).

## 1. The question

Every Mac's store is a `RemoteStore` client of the router service. `LookupHostToken` and `HostIdentity` are in the service's `notServed` set (`internal/routerstore/serve.go:59-63`), so a node cannot evaluate Negative Control 1 (token adopted to a different host than the lane's pin) or 2 (token revoked after a prior adoption). The branch therefore makes `RunInformer` refuse a `RemoteStore` outright (`ErrInformerRemoteBackendUnsupported`, commit `4d066f1a`), and the host-pin predicate `InformerHostAuthorized` exists only on `*SQLiteStore` (`internal/routerstore/hosttokens.go` on the branch, commit `e1c58c37`). Result: the informer can run nowhere a Mac actually runs, and SSA ruled `20261007-230828` that a refusing `RemoteStore` client cannot count as the M1 observation path.

## 2. Two facts that frame every option

1. **The informer has two jobs with different homes.** *Deciding and observing* (which edge, is this caller allowed) can live at the service. *Actuating* (take the per-agent PID-file lock, spawn the consumer, write the spool directory, send the session message) is host-local by nature; a Cloud Run service cannot spawn a process on a Mac. Sprint tasks 4, 6 and 8 are all actuation (`admission.go` flock, `SetWake` marker, pidfile and census row).
2. **Every served call already passes the full chain on the service**: bearer host token, session, nonce, signature, runtime hash, ownership, lane scope for scoped tokens (`serve.go:~565-583`, step 6a). A revoked token already gets 401 on its next call. The informer's only input from the service is `Wait`/`ListenNotify` (`internal/routerstore/remote.go:676`, `:690`, a 25 s long-poll), which is a served method.

## 3. Options

| | (a) Literal service-side informer | (a') Service-gated informer (recommended) | (b) Dispatch-authorization RPC | (c) SQLite-local only |
|---|---|---|---|---|
| Shape | informer process runs at the service for all lanes | node-side informer (the branch code) over `RemoteStore`; the **service** applies the Decision-6 predicate inside served `Wait` for a lane | new served verb the node calls before each push; host stamped from the session | informer runs only where the store is a local SQLite |
| NC1 / NC2 | enforced at the service | enforced at the service, per long-poll (NC2 within one 25 s poll, and on every other call) | enforced at the service, but the node chooses whether to ask | enforced locally, but only on the one host that owns the DB |
| New credential surface | none | none (existing token and session; no new verb) | none strictly, but a new served verb and a new oracle to review; the ADR classes it a security decision | none |
| Can it actuate on a Mac? | **no.** It still needs a node actuator, so it does not remove node code, it adds a second place | yes (unchanged node actuator) | yes | yes, on one host |
| Reuse | little: new service process, new supervision on Cloud Run where A32 pidfile and A33 census do not apply | most: branch tasks 3-6 and 8 as written; `ListenNotify`/`Wait` as served today | branch tasks 3-6, 8, plus new verb and client | the branch as it stands |
| Security weakness | none new | a compromised node ignores the check, but it also holds its own token, so no authority is added; the check stops misconfiguration and cross-host reuse, which is the incident class | advisory: a compromised or misconfigured node can skip the call and spawn anyway (the same limit as (c)) | none new, but it covers no production Mac |
| Effort (estimate, hours of agent work) | 20+ plus redesign of tasks 4, 6, 8 | 6 to 10 | 8 to 12, plus a security review cycle | 0 more, but it does not meet the goal |

Estimates are mine, unmeasured.

## 4. Challenging the "service-side, no credential surface" leaning

The leaning is right about the goal (no new credential) and wrong if read literally.

- **A literal service-side informer does not remove the node.** Delivery to a Mac lane is a spawn, a spool write or a session message on that Mac. So (a) leaves a node-side actuator in place and adds a service-side watcher beside it. That is the "second dispatch authority" ADR-065 rejects (alternative 4) in a milder form, and it makes the shared PID-file admission boundary of sprint /goal item 6 unreachable from the service.
- **A32/A33 supervision does not fit Cloud Run.** Decision 6 requires pidfile recognition and a census row; a service goroutine has neither, so informer absence (NC3) would need a different, new mechanism.
- **The part of "service-side" that is correct is the authorization.** The service is the only place that can read `LookupHostToken` and `HostIdentity`, and it already evaluates the chain on every call. The smallest move is to put the Decision-6 predicate there, on the call the informer already makes, rather than to add a verb (b) or move the actuator (a).
- **(b) is not as bad as the ADR text implies, but it buys little.** If the host is stamped from the session (the existing `AdoptTokenMachineID` pattern, `serve.go:~170-178`), a verb returning allow/deny for the caller's own token leaks nothing about other tokens. Its weakness is the one every node-side check has: the node decides whether to ask. (a') gets the same refusal without that gap, because a node denied `Wait` receives no edges.
- **What (a') does not do.** It does not stop a node from spawning a consumer for a lane it already knows about without ever polling. That is outside Decision 6's threat model (a host acting on its own lanes with its own token) and is the same exposure `RunWakeLoop` has today.

## 5. Recommendation

Adopt **(a')**, stated as: *the service gates `Wait`/`ListenNotify` for a lane on the Decision-6 predicate (token live, adopted, adoption equals the lane's pinned machine id), using server-stamped `sess.Host` and `HostIdentity`; the node-side informer from the branch runs unchanged over `RemoteStore`; the `RemoteStore` refusal is removed.* This keeps Ra's no-new-credential intent and drops the unachievable literal reading. Keep (c) as the in-process path for the host that owns a SQLite store.

Two things (a') assumes and this note has **not** verified: that a lane-to-pinned-machine-id source exists server-side (the sprint calls it the lane's "pinned node identity", rs-42; I did not locate where it is stored), and that `Wait` on a multi-instance Postgres-backed service wakes across instances (not examined).

## 6. What code remains for Phase-1 tasks 3-8 under (a')

State first: of tasks 3-8, **none is on `main`.** Tasks 3, 4, 5, 6 and part of 7 are written on `origin/ra/rs37b-informer-host-wiring` and unmerged; the rest is unwritten. The branch has not been re-reviewed or re-run for this note.

| task | on the branch (unmerged) | remaining under (a') | files |
|---|---|---|---|
| 3 type-to-strategy table + narrow-not-widen test | `DeliveryStrategyFor`, commit `e4c13cc8` | merge only | `internal/router/informer.go`, `internal/router/informer_test.go` |
| 4 shared atomic admission | commit `28ca5740` (`admission.go`, unix/windows lock files, `wake.go` refactor) | merge only; confirm the existing `RunWakeLoop` tests pass unmodified | `internal/router/admission.go`, `admission_unix.go`, `admission_windows.go`, `admission_test.go`, `wake.go` |
| 5 subscriber loop + NC6 coexistence test | `RunInformer`, commit `c5aafe6f` | **delete the `RemoteStore` refusal** (`4d066f1a`) and its test; prove `ListenNotify` over `RemoteStore` drives it | `internal/router/informer.go`, `internal/router/informer_test.go` |
| 6 delivery-attempt marker, NC5 | `markDeliveryAttempted`, commit `132ab3de`, via `dispatch.OpenRoot` and `SetWake` | **verify it works when the store is remote**: `OpenRoot` may open a local store, which would write the marker in the wrong place; fix if so | `internal/router/informer.go`; possibly `internal/dispatch` |
| 7 Decision-6 validation, NC1 + NC2 | `InformerHostAuthorized` on `*SQLiteStore` (`e1c58c37`) | **new:** call the predicate inside the served `Wait` (and the lane-notify entry point) for the stamped host and the lane's pin; NC1 and NC2 become service-level tests; remove or repurpose the node-side predicate | `internal/routerstore/serve.go`, `internal/routerstore/hosttokens.go`, new `internal/routerstore/*_test.go` |
| 8 census row, load-bearing pidfile, NC3 + NC4 | none | **new:** informer `censusMatchers` row, pidfile in `LoadBearingPIDs`, doctor finding for absence, misaddressed delivery surfaced as unreachable | `internal/router/census.go` (`censusMatchers`, line 79), `internal/guard/loadbearing.go` (`LoadBearingPIDs`, line 116), and the `doctor` finding site (file not located in this check) |

Also remaining and outside tasks 3-8: task 9 (changelog fragment and the ADR-065 status note), the bounded 60-minute M1 observation, and a validated completion proof per the sprint. The sprint's own gate stays: all six negative controls shown red before green.

## 7. Open decisions

1. Adopt (a') as the answer to ADR-065 condition 4 (Ra), then SSA review of the served-method change.
2. Whether gating an existing method counts as the "credential surface" security decision the ADR reserves for the owner. My reading is no (no new credential, no new verb), but that reading is Ra's, not the owner's, and the ADR wording leaves room to disagree.

## 8. Addendum 2026-11-25: the lane-to-Mac pin source now exists (report-only)

**Status:** built on `ra/lane-machine-pin`, not merged or deployed. **This does not complete rs-37**, and it does not adopt (a'): it supplies the one input §5 said I had not located. Owner decision 2026-11-25: the router keeps a trusted, server-side record of which Mac each lane belongs to; Ra-managed (set only by the router operator, a lane cannot change it); **report-only at first, blocks nothing until the owner turns enforcement on.**

**Source, and what it is not.** Schema v30 table `lane_machine_pins(agent, machine_id, set_by, reason, created, retired)`. It is not derived from a thread record's `host`, which a session asserts about itself (ADR-067 §2 rejected shape-based inference for the same reason). The pin is written only through `sirsi router lane-pin set|retire|list` against the service's own backend, the same surface as `router token mint|revoke`. `SetLanePin`, `RetireLanePin`, `ListLanePins` and `ActiveLanePinMachines` are in `notServed` and `spoolRefused`: no lane, bootstrap token, scoped token or relay can call them. `list` is operator-only too: pin rows name machine ids and who set them, and nothing needs a lane to read them, so I did not serve it.

**Shape.** A lane may hold several active pins (some run on more than one Mac). A pin is retired (`retired` = timestamp), never deleted, so history is kept; a partial unique index forbids two active rows for one `(agent, machine_id)` and does not block re-pinning after a retire. No backfill: the upgrade pins nothing.

**Evaluation (Decision 6, NC1).** `EvaluateLanePin(callerMachineID, activePins)` returns `match`, `mismatch`, `unpinned` (no active pin: nothing to compare) or `no-machine-id` (lane pinned, token has adopted none, or it is the bootstrap token). The caller's machine id is `host_tokens.machine_id` of the bearer the request authenticated with, read by the lookup `hostForBearer` already does, so it costs no extra query. At a served `Wait` only, the verdict is counted; the request proceeds identically for every verdict. NC2 (revocation) is untouched: a revoked token already fails authentication before this point.

**Report-only, bounded.** Counters are in memory per service instance (lane label x verdict); every lane with no active pin shares one `_unpinned` label and a full lookup cache counts `_overflow/skipped`, so a caller inventing lane names cannot mint keys (hard cap 512). A failed or panicking evaluation is counted as `error` and returned to nobody. The counters are readable as one log line every 5 minutes (`lane-pin (report-only ...)`); there is deliberately no new endpoint, and they reset on restart.

**Cost on the hot path.** One indexed read of the lane's active pins (the primary key's `agent` prefix) on a cache miss, then a 60 s cache per lane (10 s after a failed read). With about 25 s long-polls that is at most one extra read per lane per minute per instance. Consequence: a pin set or retired by the operator is seen by a running instance within 60 s, and across Cloud Run instances each keeps its own cache and counters.

**The enforcement seam.** `lanePinRefuses(verdict) bool` in `internal/routerstore/lanepinobserve.go` returns false for every verdict, and the call site discards the result. Turning enforcement on is a separate, owner-gated change to that one function and its wiring; there is no flag, env var or switch to flip today. The mismatch counters are the evidence of what it would have refused, and should be read before that decision (an unpinned or `no-machine-id` lane today means "not yet pinned or adopted", which enforcement must not treat as a mismatch without a deliberate choice).

**Still open for rs-37.** (1) Operator must pin the lanes and confirm each Mac token has adopted its machine id (`AdoptTokenMachineID`), or every verdict reads `no-machine-id`/`unpinned`. (2) §5's second assumption (multi-instance Postgres `Wait` wake) is still unexamined. (3) Whether gating a served method on this predicate is the owner's "credential surface" decision (§7.2) is unchanged; the report-only build adds no refusal.

## 9. Addendum 2026-11-28: (a') adopted, part A shipped, part B pending

**Decision (Ra, design owner):** option (a') from §5 is adopted for the service-side predicate. The service refuses a served `Wait` for a lane whose token's adopted machine id differs from the lane's operator-set pin; the node-side informer runs over `RemoteStore` unchanged. No new verb, no new credential. SSA review of the served-method change and the owner's answer to §7.2 are unchanged and still open.

**Part A shipped: #1463 (`f5cf280f`).** `SIRSI_ROUTER_LANE_PIN_ENFORCE` (service-side, read once at start, default off, non-boolean is a startup error). This supersedes the "no flag" statement in §8: the seam `lanePinRefuses` now returns true only for `enforce && mismatch`; unpinned, `no-machine-id`, error and skipped are always allowed. **Enabling it is an owner decision**, taken after the operator has pinned the lanes and every Mac token has adopted its machine id; before that the flag would refuse nothing.

**Part B pending, not started:** node-side `RunInformer`, shared admission, delivery-attempt marker, removal of the `RemoteStore` refusal (§6 tasks 3 to 6), and the census and load-bearing rows (task 8). Part C (this change) is docs only.

**Negative-control coverage as of this date**

| NC | Service-side coverage | Where |
|---|---|---|
| 1 mismatched token host | covered for the served `Wait` predicate: flag on refuses exactly a mismatch (403, `ErrLanePinMismatch`, names lane and both machines, no token), flag off allows it, match passes with the flag on (positive control) | `TestLanePinEnforcementAtServedWait`, `TestLanePinRefusesTable` in `internal/routerstore/lanepins_test.go` |
| 2 revoked after prior adoption | not covered by #1463's tests. Revocation is enforced by the authentication chain before the predicate runs, and `RevokeHostToken` kills every session of the token (pre-existing `TestNegativeControlRevokeHostTokenKillsEveryHostSession`). The informer-level NC2 test belongs to part B | `discardtoken_test.go` |
| 3 to 6 | not covered; they need the informer process | part B |

**Lane-pin mismatch counts have no read surface.** The report-only counters (`lanePinObserver.snapshot`) live in service memory, per instance, and are emitted only as a log line every 5 minutes (`lane-pin (report-only ...)`). No served method exposes them, so a `doctor` finding "enforcement would refuse N" needs a new served verb. That is out of scope for part C; until one exists, read the service log line before enabling enforcement.

**Condition 4, remaining question.** Whether a served-method gate on this predicate is the owner's reserved "credential surface" decision (§7.2). Ra's reading is no; it stays open until the owner answers or SSA rules.
