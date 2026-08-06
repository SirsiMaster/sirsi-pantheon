# Agent Operations

`sirsi ops` shows the operator-facing paths for work an AI agent would normally
do behind the scenes: checking the router, reviewing files, asking the local
model, syncing memory, watching thread health, reaping stale sessions, and
diagnosing host state.

The rule is simple: deterministic command first, local AI optional. `--json`
prints the same map for menus, tests, and automation.

```bash
sirsi ops
sirsi ops --json
```

## What It Shows

Each row names:

- the operation,
- the deterministic CLI command,
- the local Ask Sirsi / Gemma path when it exists,
- the menubar surface expected to expose the same work.

This makes Agent-Operations Parity inspectable. If a command exists but the
menubar does not expose it yet, the row is the product gap.

## Common Paths

```bash
sirsi ctr
sirsi ctr --reconcile
sirsi router status
sirsi router plan
sirsi router node-status
sirsi thread discover
sirsi thread prune
sirsi thoth sync
sirsi gemma "Summarize the open router queue"
```

## Safety

`sirsi ops` is read-only. It does not wake agents, close router items, clean
files, or modify thread registries. Use the commands it lists when you want to
act.
