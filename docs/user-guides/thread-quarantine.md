# Thread quarantine

**One-line:** park stale thread records reversibly, so they stop counting as live or stale, without ever reaping, closing, pruning or deleting them.

Owner rule: threads and sessions are never reaped, pruned, closed, archived or deleted by an operator sweep. `sirsi router doctor --fix` reaps, so it is not the answer for a pile of stale records. Quarantine is the non-destructive alternative.

## Commands

```sh
sirsi thread quarantine --stale --dry-run     # list what would be parked; writes nothing
sirsi thread quarantine --stale               # park every currently stale thread owned by this host
sirsi thread quarantine --thread <id>         # park one thread (refused unless it is stale)
sirsi thread quarantine --stale --stale-after 6h --reason "post-incident sweep"
sirsi thread unquarantine --thread <id>       # release one manually
sirsi thread list --quarantined               # see what is parked
```

`--stale-after` defaults to 1 hour (the 5-minute display window of `thread list` is deliberately more eager than what is safe to park). There is no force flag: a thread with a fresh heartbeat, a live process, or a live watcher loop is refused. Each run prints a table of what changed (or would change with `--dry-run`); `--json` gives the same rows.

## What quarantine means

- The record stays in the registry. Only `status` becomes `quarantined`, and a `quarantine` note is added: who, when, why, prior status. `last_seen_at` is preserved.
- It is excluded from live and stale counts (`router node-status`, doctor, board, ledger fabric health), from attended-session detection, from "armed" credit, and from wake targeting.
- It is never reaped, superseded or pruned by any sweep.
- `sirsi router doctor` and `router node-status --json` (`quarantined_threads`) list it separately; `sirsi thread list --quarantined` shows the rows.

## Release

A heartbeat or re-register from the same thread releases it automatically, restores its prior status and logs `released from quarantine: proved alive`. A live session is never cut off. `sirsi thread unquarantine` restores the prior status without touching `last_seen_at`, so a thread released by hand still reads stale until its session heartbeats.

## Limits

- Only records owned by the host running the command are changed. Records owned by another host (for example the `cloud-board` instances) are reported as `skipped-foreign-host`; the service refuses cross-host writes.
- Writing the status needs a router service that accepts an equal-timestamp write into `quarantined`. Deploy the service before relying on the CLI against a cut-over store.

## Cloud board threads

`sirsi router board-serve` registers `router-board` and closes its own thread on SIGTERM (A27). The close now completes before the process exits.
