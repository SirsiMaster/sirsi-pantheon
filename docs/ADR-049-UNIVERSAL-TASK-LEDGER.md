# ADR-049: Router Universal Task Ledger

## Status

**Implemented and independently reviewed; Pantheon ratification pending** — 2026-08-02

**Owner directive:** keep the machine in a two-agent quiet regime while the SNE
backend is completed; codex-inference builds the ledger, Claude Nexus reviews,
and Claude Pantheon ratifies after the quiet regime.

## Context

The router can prove that a message is open, but it cannot answer the owner's
next operational questions in one place: how old is it, is its assignee still
heartbeating, is it blocked by another item, has a live thread picked it up, and
what larger task does it represent? CTR therefore reports counts without a
complete execution ledger. Model memories then become competing, incomplete
accounts of portfolio state.

## Decision

Ra owns one Universal Task Ledger read model. It joins three existing truths;
it does not replace or duplicate them:

1. Router items remain the message and evidence record. They gain one optional,
   losslessly mirrored `blocked_by` item-id edge.
2. The durable router SQLite store gains a per-agent task registry with the
   fixed columns `{agent, task-id, subject, status, responsible-party,
   blocked_by, created, updated}`. Status is one of `pending`, `in-progress`,
   `blocked`, or `done`.
3. The thread registry remains the authority for heartbeat freshness and
   `current_item` pickup evidence.

`internal/ledger` is the sole join/interpretation layer. `sirsi router ledger
[agent]` renders its full text or JSON projection. CTR renders the same model's
per-agent oldest age, stale flag, blocked count, and unblocked/unpicked count.
`sirsi router task add|update|list` manages task rows. `sirsi router depend`
sets or clears an item dependency.

An open item is stale when its assignee has no non-terminal heartbeat, or its
newest heartbeat is at least four hours old. The default is explicit and the
ledger CLI exposes `--stale-after` for a different operating window. Missing or
cyclic dependencies fail closed as blocked. A terminal dependency releases its
dependent item. Pickup requires a non-terminal thread whose `current_item`
equals the item id; a fresh heartbeat alone is not pickup evidence.

### Data flow

```text
router items + task registry (SQLite) ─┐
                                      ├─ internal/ledger ─┬─ router ledger
thread heartbeats/current_item ───────┘                   └─ CTR summary
```

### Implementation order

1. Add migration v3 and lossless item dependency projection.
2. Add validated task CRUD and deterministic listing.
3. Add the shared ledger read model and dependency resolution.
4. Add CLI verbs and CTR projection.
5. Seed Claude Nexus's task box through the new verbs as the first live
   integration test after independent review. The original 32-task SNE
   snapshot became a truthful 35-task whole-agent registry when three later,
   router-grounded obligations were included; no padding was invented.
6. Ratify through Claude Pantheon and extend thin UI surfaces later.

### First live integration and rollout invariant

Claude Nexus independently approved exact implementation head `b08bac6d` and
published the first live registry. Independent replay confirmed all 35 rows,
the status and responsible-party totals, and every populated dependency target.

The first live migration also exposed an operational defect outside the read
model: a shadow v3 binary migrated the shared router database before the
canonical installed v2 binary was replaced. The older binary correctly failed
closed, but router availability was temporarily lost. Recovery required a
validated v3 build followed by an atomic new-inode replacement of the installed
binary. Future schema rollouts must therefore upgrade the canonical executable
atomically before any new binary opens and migrates the shared live store. A
schema migration is not operationally complete until all host router surfaces
can reopen the migrated store.

### Key decisions

| Question | Decision |
|---|---|
| New database or existing router store? | Existing store; task state belongs beside dispatch truth and remains offline-first. |
| Infer pickup from heartbeat? | No; only `current_item` proves pickup. |
| Missing dependency? | Blocked, fail closed; invisible prerequisites must not look runnable. |
| Web UI in v0? | No. CLI/JSON and CTR are the authoritative first surfaces. |
| Cross-machine sync? | No. This version is local-node truth; portfolio synchronization remains Ra's later scope. |

## Alternatives Considered

1. **Use model memory as the task registry** — rejected because memory is
   partial, per-model, and not transactionally tied to router state.
2. **Encode tasks only as router messages** — rejected because messages and
   execution commitments have different lifecycles; overloading item status
   recreates ambiguity.
3. **Build a dashboard first** — rejected because a UI without one typed read
   model would multiply conflicting aggregation logic.
4. **Treat every fresh thread as having picked up work** — rejected because CTR
   already established that heartbeat-fresh does not mean inbox-consuming.

## Consequences

- **Positive:** the owner and every agent can inspect one deterministic account
  of open work, dependencies, staleness, pickup, and responsibility.
- **Positive:** CTR gains prioritization signals without becoming a second
  ledger implementation.
- **Positive:** item export/backfill remains lossless across SQLite and Markdown.
- **Negative:** task status is an explicit discipline; agents must update it.
- **Risk:** a stale `current_item` can overstate pickup until its thread is
  reaped. Existing OS-truth reaping remains the mitigation.
- **Risk:** the four-hour default may not fit every workflow; it is visible and
  configurable rather than hidden.
- **Risk:** automatic forward migration can strand older host binaries. Release
  procedure must treat canonical binary installation and shared-store migration
  as one ordered operation, with a preflight before live-store mutation.

## References

- ADR-017 — Ra/Horus CTR Hypervisor
- ADR-024 — One Watcher Per Surface
- ADR-026 — Horus Ops Dashboard
- ADR-036 — Router v2 Durable Dispatch
- ADR-038 — Universal Surface
- Owner router item `20260802-165804-claude-nexus-codex-inference-owner-order-two-agent-quiet-regime-until-sne-backend-done-yo`
