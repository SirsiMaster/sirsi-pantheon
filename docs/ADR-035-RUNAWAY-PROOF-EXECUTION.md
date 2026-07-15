# ADR-035: Runaway-Proof Execution — Fenced Dispatch Authority + Sekhmet's Host Backstop

## Status
**Accepted** — 2026-07-04 · claude-pantheon; the dispatch axioms were adversarially bounced with codex-SME (round 1 FAIL-as-stated, round 2 **APPROVE**, session 019f2f6c-207f) and bound as the Phase-2 Dispatch Contract (PRD `docs/prd/ROUTER_V2_DURABLE_DISPATCH.md` §2b, PR #160). Custodians: 𓁢 the Router (dispatch authority) + 𓁵 Sekhmet (host backstop). Companion to the ADR-031 family: 031-A/B/C govern *memory* exhaustion; this ADR governs *executor* exhaustion — sessions, queue items, and disk.

## Context

Two incidents, one disease (full forensics: `docs/case-studies/2026-07-04-runaway-executor.md`):

1. **2026-07-03** — a build-worker LaunchAgent spawned a full headless agentic session for every open router item and closed none: **19,195 sessions, 0 completed**, infinite reprocess, six weeks of misdiagnosed "arming" pain.
2. **2026-07-04** — the bounded-retry stopgap's non-idempotent give-up path flooded **11,564 escalation items in one night**; and the worker's timeout-killed `go test` runs orphaned **~1.3 TB of build trees in 36h**, filling the disk to 100% (ENOSPC) with **zero alarms** at any layer.

The structural fact under both: *any* executor wired around a fenced dispatch path — file writes, raw sends, sidecar workers — relocates the failure mode instead of removing it. And a correct dispatch layer alone is not enough: the host must be able to detect and stop a runaway executor **independently of the layer that is misbehaving** (the ADR-031-A defense-in-depth precedent, applied to execution).

## Decision

Six axioms. The first four are the architecture-level statement of the binding Phase-2 Dispatch Contract (§2b implements them verbatim); the last two are the host's independent defense.

1. **One executable dispatch authority.** The routerstore is the ONLY path to executable work: lifecycle is `open → claimed → working → blocked | dead_letter | completed`, terminal states are terminal, mutations are atomic and lease-token-fenced, and **claim is the only door to execution** (pull stays read-only). "Give up" is a `dead_letter` with owner metadata — never an item left open forever.
2. **Idempotency precedes autonomy.** Any automated path that emits items (sends, escalations, give-ups, throttle notices) MUST be idempotent — keyed singletons, update-in-place, bounded counters — *before* it is allowed to run unattended. The 11,564-item flood is the standing proof that a non-idempotent stopgap inherits the runaway it patches.
3. **Bounded everything.** Budgets on concurrent claims, sends per window, retries per item, and total active work; circuit breakers per failure domain that pause dispatch and write ONE bounded operator item. N distinct failures ≠ N escalations.
4. **One red number.** Dispatch observability (claims, lease expiries, retries, rate-limit drops, dead letters, breaker state) surfaces in `node-status` as an aggregate. The next incident must be a single alarming counter, not a directory of 11,564 files.
5. **The host defends itself (Sekhmet's backstop).** Independent of the dispatch layer, 𓁵 Sekhmet watches the two live signatures of a runaway executor — headless agentic-session flood and fresh (<24h) build-tree churn in the per-user temp dir — as the doctor's **"Runaway Executor"** finding, alarming only on a current, fixable condition (surfaces canon) and carrying a real lever per ADR-033: **`sirsi router quarantine-worker`**, which boots out every `ai.sirsi.claude-worker.*` LaunchAgent and renames its plist to `.plist.quarantined` (durable across login, hand-reversible). Wake-loop watchers and the router supervisor are **never** touched — in both incidents the watchers were healthy; the executor was the disease.
6. **Re-arm is gated, not casual.** The claude build-worker stays OFF until the §2b acceptance bar passes: fenced lifecycle ops tested, facade idempotency + quotas enforced before insert, keyed-singleton escalations tested, breakers proven, and safety tests that **reproduce both incidents** (duplicate-claim race; stuck item ⇒ at most one terminal record; sender flood rejected; restart mid-lease; expired lease cannot complete newer-leased work).

## Alternatives Considered

1. **Patch the worker script harder** — Rejected: the stopgap already proved that patching a runaway system without idempotency multiplies the failure (11,564 items). The fix must live in the dispatch authority, not the caller.
2. **Rely on the hourly reaper alone (#161)** — Rejected as sole defense: it drains >24h-old artifacts (symptom relief) but is deliberately blind to the first 24 hours and raises no alarm; 1.3 TB accumulated in 36h. Detection must fire *while* the disease runs.
3. **Let the doctor auto-kill runaway processes** — Rejected: the doctor is read-only by design; remediation is explicit, consented, and labeled honestly (ADR-033 three-outcome law). The check names the disease; the operator (or a supervised surface) pulls the one-command lever.
4. **Quarantine by deleting the worker plists** — Rejected: rename (`.plist.quarantined`) is equally durable against RunAtLoad/KeepAlive resurrection but stays hand-reversible for the gated re-arm (Axiom 6), and preserves the definition as evidence.

## Consequences

- **Positive:** the runaway-executor class is now (a) structurally impossible through the store (fenced, idempotent, bounded), (b) detectable at the host within one doctor pass instead of at ENOSPC, and (c) stoppable with one safe command that cannot collateral-damage the fabric's watchers. The six-week "threads never stay armed" failure mode has a named root cause and three independent guards.
- **Negative:** until Phase 2 lands, the build tier runs at reduced autonomy (gemma triage stays; claude builds happen in supervised sessions). This is deliberate — Axiom 6 makes restoring autonomy conditional on the acceptance bar, not on impatience.
- **Enforcement:** Ma'at treats an executor wired around the store, a non-idempotent automated emitter, or a worker re-armed before the acceptance bar as governance failures. Regression tests: `internal/router/workerquarantine_test.go`, `internal/guard/runaway_test.go`, and the §2b safety suite (Phase 2).
