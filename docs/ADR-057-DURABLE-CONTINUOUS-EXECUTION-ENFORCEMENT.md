# ADR-057: Durable Continuous-Execution Enforcement

## Status

**Accepted by owner directive; implementation and production acceptance in progress** — 2026-08-05

## Context

Prompt instructions, periodic heartbeats, and process existence repeatedly made an
agent look alive while durable work remained untouched. This failure is
provider-neutral: Codex, Claude, local models, and future workers can all finish a
turn, retain a generic current-item marker, or heartbeat without mutating work.
Continuity therefore cannot be a behavioral promise made to a model.

ADR-039 defines continuous work and honest gates; ADR-050 defines the universal
ledger; ADR-052 defines durable conduit semantics. This ADR makes their joint
execution mechanically enforceable in Pantheon's Go runtime.

## Decision

Pantheon owns one durable supervision state machine for every registered agent
lane, independent of model provider or UI surface.

### 1. Authoritative runnable predicate

```text
runnable =
  actionable router item
  OR actionable ledger task
  OR unmet traced canonical requirement
  OR canonical registry requiring its first audit
```

A lane may be classified `COMPLETE` only when this predicate is false and the
completion integrity checks pass. An empty inbox is never sufficient.

### 2. Transactional claims

Workers follow `pull -> lease -> bind thread/task -> execute -> record evidence
-> close -> pull again`. Router and task claims carry a lease token, expiry,
attempt count, worker identity, thread identity, and idempotent source identity.
Expired claims are reclaimed. Completion without an evidence reference fails.

### 3. Durable event wake

Creation or unblocking of an item/task, discovery of a canon gap, lease expiry,
and completion while more work remains emit durable, idempotent wake events.
Wake delivery uses bounded retries and terminal-failure escalation. Invoking a
process is not acknowledgment: only a real store mutation (router lease, task
lease, or recorded requirement audit) may acknowledge delivery.

### 4. Honest operational states

Horus and every supervisor surface use the same classifier:

- `WORKING`: recent durable work mutation under a valid lease.
- `ASSIGNED`: valid lease exists but no recent mutation yet.
- `IDLE_WITH_WORK`: runnable work exists, no lease/mutation, wake path exists.
- `BLOCKED`: only recorded technical, security, privacy, or dependency work remains.
- `UNROUTABLE`: runnable work exists but no valid wake path exists.
- `COMPLETE`: all three sources and all integrity gates are terminally verified.

Heartbeat and PID state are diagnostic inputs only. They never prove work.

### 5. Mechanical reconciliation

The resident Go supervisor periodically reconciles router items, task leases,
thread bindings, requirement records, wake events, and completion evidence. It
expires orphan leases, creates missing requirement tasks, rejects false `done`
states, repairs/flags unroutable lanes, and re-wakes idle lanes. A worker that
finishes one item while more work exists receives a continuation event.

### 6. Evidence-backed completion

A satisfied canonical requirement must reference its requirement ID,
implementation commit or PR, tests, security control, design acceptance,
deployment version, and production verification. Waivers are explicit,
reasoned terminal dispositions. Dead letters, terminal wake failures, blocked
work, unmet requirements, unaudited canon, and post-cutover proofless `done`
records all prevent `COMPLETE`.

### 7. Cross-provider enforcement boundary

Adapters may differ for Codex.app, Claude, local inference, CLI workers, and
future providers, but they implement only wake delivery. The runnable predicate,
leases, acknowledgments, reconciliation, state classification, and completion
gate remain in provider-neutral Go/store code. A model prompt cannot override
them. Codex.app heartbeats remain a compatibility bridge until its registered
task wake adapter proves store acknowledgment.

## Acceptance Criteria

1. Store migrations are backward-safe and the canonical installed binary is
   upgraded before any live-store migration.
   The shared `~/.sirsi/router.db` may advance only during an explicit atomic
   deployment with `SIRSI_ALLOW_SCHEMA_MIGRATE=1`; ordinary working-tree
   binaries fail closed. Schema advancement itself requires the same reviewed
   commit, deployment-version, rollback-backup, and production-verification
   evidence as any other completion claim.
2. Adversarial tests prove heartbeat-only activity cannot yield `WORKING`, an
   arbitrary acknowledgment cannot consume a wake, expired leases recover, and
   proofless completion is rejected.
3. The resident supervisor wakes `IDLE_WITH_WORK`, reports `UNROUTABLE`, and
   exposes the shared classifier through Horus.
4. Independent exact-head review passes.
5. The canonical binary is atomically installed and a production exercise
   proves: create work -> wake -> store claim -> evidence close -> continuation
   or verified complete.
6. A requirement-by-requirement proof artifact links code, tests, security,
   design, deployment, and production evidence.

## Consequences

- **Positive:** “never stop while work exists” becomes a recoverable system
  invariant across all LLMs instead of an unenforceable instruction.
- **Positive:** Horus distinguishes session liveness from actual work and makes
  idle-with-work and unroutable lanes visible.
- **Negative:** more durable state, migrations, leases, and operational tests.
- **Risk:** a broken wake adapter can still prevent execution; it is surfaced as
  `UNROUTABLE`, retried within bounds, and cannot masquerade as healthy work.
- **Risk:** schema rollout can strand older binaries; ADR-050's atomic upgrade
  ordering is mandatory.

## References

- ADR-039 — Continuous Wake-and-Work Surface
- ADR-050 — Router Universal Task Ledger
- ADR-052 — A2A/Router-Conduit Operating Rules
- ADR-054 — Contracts, Identity, and Ledger v7 / One Horus
