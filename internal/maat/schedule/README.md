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
  (via `GetState`/`SetState`), so an M1 reservation is visible on the M5 and to
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

- **Grant is a read-modify-write** on one state key; two hosts reserving the
  *same* resource within the few-ms RMW window could both win. The reservation
  cadence is a handful per hour, so this is negligible; upgrade to a
  compare-and-swap on the state key if throughput ever demands it.
- The default `ActivityProbe` is a process classifier. **Per-rail traffic
  counters** (attributing load to a specific cable) are a deeper detector that
  plugs into the same interface — framework built, rail detector is a follow-up.

## Owner gates

Unchanged: no credentials, no kernel/SIP/sysctl changes. The scheduler only
admits or defers work.
