# Runaway Executor Guard — detect and stop a runaway automation

**Deity:** 𓁵 Sekhmet (the Warrior — watchdog)
**Canon:** ADR-035 · case study `docs/case-studies/2026-07-04-runaway-executor.md`

## What it does

If an automation on your machine starts spawning AI sessions or build runs faster
than it finishes them, Pantheon now notices **while it is happening** and gives you
one command to stop it.

`sirsi doctor` (and every surface that reads it — menubar, dashboard, the
SessionStart health line) includes a **Runaway Executor** check. It watches two
live signals:

- **Headless agent sessions** — background `claude -p` / `claude --print`
  processes. Your own interactive session is never counted.
- **Fresh build trees** — `go-build*` / `sirsi-integration-*` folders under 24
  hours old in the system temp directory. (Older ones are already cleaned up
  automatically every hour.)

A handful of either is normal and the check stays green. Dozens of sessions or
hundreds of young build trees means something is running away, and the finding
turns yellow (or red) **with a fix attached**.

## The fix: `sirsi router quarantine-worker`

```
sirsi router quarantine-worker            # stop the worker tier, durably
sirsi router quarantine-worker --dry-run  # show what would be stopped, change nothing
```

What it does, in order:

1. Stops every loaded claude build-worker background job (`ai.sirsi.claude-worker.*`) immediately.
2. Renames each worker's launch file to `….plist.quarantined` so it cannot come
   back at login. To re-arm later, rename the file back by hand — deliberately a
   human step, because per the Dispatch Contract the build worker stays off until
   its safety bar passes.

What it will **never** touch:

- Wake-loop watchers (`ai.sirsi.router.wake.*`) — the agents' inbox listeners stay alive.
- The router supervisor — dispatch, sweep, and registry care keep running.

It is safe to run twice: a second run finds nothing and says so.

## Why this exists

On 2026-07-03/04 a build worker spawned 19,195 sessions (finishing none), flooded
the work queue with 11,564 items, and filled 1.3 TB of disk with orphaned build
trees — and nothing alarmed until the disk hit 100%. The full story, and the
architecture that makes it impossible to repeat, is in the case study and ADR-035.
