# `internal/maat/schedule` — the Ma'at metal reservation scheduler

`stacklab.wing.maat`, gate **MAAT-WING-001.G1**. Enforced admission and
de-confliction of shared machine + Thunderbolt-rail usage, so lanes cannot
contaminate each other's measurement windows. Replaces the advisory `rails.lock`.

## Why

Two rail collisions in one day (claude-io, 2026-09-24): unregistered work ran
bench traffic on shared rails inside other lanes' measurement windows, and
self-hosted CI runners fired mid-measurement. `rails.lock` was a convention, not
a control.

## Design

- **Cross-host by construction.** The ledger lives in the shared router store
  (via `GetState`/`CompareAndSwapState`), so an M1 reservation is visible on the M5 and to
  **any future Mac** — a new Mac participates by reserving its own
  machine-scoped resources, with no code change. Resources are free-form slugs
  (`m1`, `rail-a`, `ci-runners@m5`), never a hardcoded machine list.
- **Heartbeat leases, no stale locks.** A reservation expires if its holder
  stops heartbeating (`LeaseTTLSec`); expiry is swept on every read, so a
  crashed holder frees the resource automatically.
- **Enforced admission.** `reserve` refuses an overlapping foreign window and
  exits **97** (the `maat-repro-lint` convention) — or queues with `--queue`.
- **Conflict detection.** `CheckConflicts` samples current load via an
  injectable `ActivityProbe` (default: a `ps` classifier for bench/build/model
  processes — closing the old `FOREIGN_EXCLUDE` hole where `tbraw-bench`/
  `tcp-bench` were ignored); a foreign actor during a quiet/loaded window marks
  the block `invalidated`, names the intruder, and (CLI) notifies its inbox.
  Rail-specific traffic detectors plug into the same `ActivityProbe` interface.
- **Dashboard read-model.** `Coverage` computes per-resource utilization %,
  the current holder, upcoming windows, and conflicts caught in 24h — the JSON a
  dashboard tile consumes (`sirsi maat coverage <resource> --json`).

## CLI

```
sirsi maat reserve <resource> --holder --work --regime quiet|loaded|build \
                   --est-end <rfc3339> [--queue] [--json]   # exit 97 if refused
sirsi maat status [resource] [--json]
sirsi maat who-is-on <resource> [--json]
sirsi maat heartbeat <id>            # keep the lease alive
sirsi maat extend <id> --est-end <rfc3339>
sirsi maat release <id>
sirsi maat coverage <resource> [--horizon 24] [--json]
sirsi maat should-defer <machine>    # exit 97 if a runner must defer
sirsi maat conflict-check <resource> [--machine <m>] [--json]  # invalidate + notify
```

Harness hook points: `maat-repro-lint.sh` calls `reserve` before touching a
cable; `maat-bench-watchdog.sh` calls `conflict-check` per block; a self-hosted
runner calls `should-defer <machine>` before starting a job.

## Ceilings (deliberate)

- Reservation mutations compare the complete observed state (including key
  presence) with one atomic SQL statement. A CAS conflict rereads and rederives
  the full grant/mutation, at most 32 attempts. Transport errors are returned
  immediately because their commit outcome may be unknown.
- Status, coverage, and holder reads project expiry without writing snapshots.
  Mutations persist expiry under the same CAS. All historical records and
  unknown JSON fields are retained; retention is not an admission side effect.
- `SetState("maat:reservations", ...)` is refused at the canonical store so
  old clients cannot erase fenced changes. See the server-first deployment
  sequence in the user guide and ADR-062 amendment. No fallback ledger exists.
- The default `ActivityProbe` is a process classifier. **Per-rail traffic
  counters** (attributing load to a specific cable) are a deeper detector that
  plugs into the same interface — framework built, rail detector is a follow-up.

## Owner gates

Unchanged: no credentials, no kernel/SIP/sysctl changes. The scheduler only
admits or defers work.

Bare `studio` and legacy `m5-studio` name one capacity pool. Comparisons apply
at the shared engine, including server atomic admission and historical rows;
stored and returned resource/ID fields retain their original spelling. Rails and
components remain distinct. Capacity reads accept either historical key and use
the smaller positive value when both disagree. Explicit capacity setters update
both aliases. No migration, reservation deletion, preemption, or host-binding
relaxation occurs. Deploy the shared scheduler on every service replica before
claiming the shadow-pool repair operationally qualified.
