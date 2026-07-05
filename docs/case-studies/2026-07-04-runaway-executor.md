# Case Study — The Runaway Executor: 19,195 Sessions, 11,564 Items, 1.3 TB

**Date:** July 3–4, 2026
**Severity:** P0 — resource exhaustion (token burn, queue flood, disk to 100% / ENOSPC)
**Deities involved:** 𓁵 Sekhmet (Guard), 𓂀 Horus (supervisor/fabric), 𓁟 Thoth (memory), 𓆄 Ma'at (governance)
**Outcome:** worker executor stopped and gated OFF; Dispatch Contract bound (PRD §2b, codex-SME APPROVED); hourly reaper (#161); sweep world-model fix (#159); ADR-035 + Sekhmet's Runaway Executor check + `sirsi router quarantine-worker` kill switch (this PR)

---

## The Problem

For roughly six weeks, agent threads "never stayed armed" — every workstream needed a manual kickstart, items sat unread, and the machine intermittently churned. The standing theory was an *arming gap*: watchers not installed, heartbeats not durable.

The theory was wrong. The watchers were the healthy part.

## What Was Actually Happening

`~/.local/bin/sirsi-claude-worker.sh` — a LaunchAgent per agent (`ai.sirsi.claude-worker.<agent>`) — spawned a **full headless agentic session** (`claude --print`) for **every** open router item addressed to its agent, including sweep-probes and heartbeat traffic. Nothing it spawned ever closed an item.

The log, when finally read as a whole:

> **19,195 BUILD START · 0 BUILD DONE · 19,192 BUILD ERROR (rc=1)**

Because no item ever closed, every poll re-pulled the same items → infinite reprocess. That single loop explained the whole six-week syndrome: the token burn, the machine churn, and why "nothing self-cleared" no matter how carefully the watchers were re-armed.

### Symptom 2 — the stopgap flooded the queue (2026-07-04)

The first fix added a bounded-retry cap (abandon after N attempts, route a "WORKER GAVE UP" notice). The give-up path was **not idempotent** — and flooded **11,564 escalation items in one night**. A stopgap bolted onto a runaway system inherited the runaway.

### Symptom 3 — the disk (2026-07-04)

The worker's timeout SIGKILLed in-flight `go test` runs. Go cleans its temp build trees only on *normal* exit, so every killed run orphaned its tree: **7,689 `go-build*` + 7,681 `sirsi-integration-*` dirs ≈ 1.3 TB in 36 hours**, filling the 1.8 TB disk to 100%. The owner hit ENOSPC; even the session harness could not write. Nothing on the host alarmed at any point — the disease ran silent until the disk was full.

## The Fixes (proven, not claimed)

| Layer | Fix | Where |
| :--- | :--- | :--- |
| Stop the bleeding | runaway killed; retry cap + probe-skip guard; worker gated **OFF** | owner-side worker script |
| Disk | `go clean -cache` (47 GB) + purge both orphan patterns; **hourly sweep reaps both patterns >24h old** | PR #161 |
| False alarms | sweep checks the supervisor of record, not the migrated-away legacy daemon | PR #159 |
| Durable arming | `sirsi router wake-install` for all 9 local claude agents (RunAtLoad + KeepAlive) | live, verified |
| The real fix | **Dispatch Contract** — store as sole dispatch authority, fenced leases, claim-only execution, idempotent send facade, keyed-singleton escalations, breakers, budgets | PRD §2b, PR #160, codex APPROVE |
| Host backstop | 𓁵 Sekhmet's **Runaway Executor** doctor check (headless-session flood + fresh build-tree churn) + **`sirsi router quarantine-worker`** kill switch | ADR-035, this PR |

The claude build-worker **stays OFF until the §2b acceptance bar passes** — token-fenced lifecycle ops, facade idempotency + quotas, keyed singletons, breakers, and safety tests that reproduce *both* incidents.

## The Lessons

1. **Stopgaps to runaway systems need idempotency FIRST.** The retry-cap patch was correct in intent and still produced 11,564 items, because its give-up path could append without bound. Any automated escalation path must be a keyed singleton before it runs unattended.
2. **Diagnose the executor, not just the watchers.** Six weeks were spent re-arming healthy watchers because "not armed" was the visible symptom. The invisible part — what the executor *did* once woken — was the disease.
3. **The next incident must be one red number, not 11,564 files.** Observability for dispatch (claims, expiries, retries, dead letters, breaker state) belongs in `node-status` as an aggregate; a host-level Sekhmet check must alarm while the disease is running, not after the disk is full.
4. **Never claim the fabric is "self-driving" without the autonomous-close proof AND a bounded, non-looping executor.** Arming proofs measure delivery, not completion.

## Regression Guards

- `TestQuarantineWorkersStopsWorkerAndRenamesPlist` — the kill switch stops workers and *never* touches wake-loops.
- `TestRunawayExecutorSessionFloodAlarms` / `TestRunawayExecutorTreeChurnAlarms` — both incident signatures alarm.
- `TestRunawayExecutorAlarmCarriesQuarantineLever` — the alarm always carries the real lever (ADR-033's three-outcome law).
- §2b acceptance bar — safety tests must reproduce both incidents before any worker re-arms.

**Full canon:** [`docs/ADR-035-RUNAWAY-PROOF-EXECUTION.md`](../ADR-035-RUNAWAY-PROOF-EXECUTION.md) · [`docs/prd/ROUTER_V2_DURABLE_DISPATCH.md`](../prd/ROUTER_V2_DURABLE_DISPATCH.md) §2b
