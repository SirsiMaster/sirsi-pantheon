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

## Ask Sirsi stock context

The native Ask Sirsi system prompt describes the current Mac and operator without
embedding a person's identity, private portfolio, agent IDs, or assumed services.
Ra owns routing across local Horus instances; Hermes is the protocol and Mercury
the transport. Ma'at is the default local intelligence and evidence surface;
Seshat remains legacy ingestion compatibility behind it. Optional models do not
establish operational authority or prove execution.

Runtime facts, model identity, agent IDs, and portfolio membership require actual
explicit context and applicable canon. Canon describes roles and plans; it is not
live telemetry or permission to act. This stock-text correction does not change
Ask Sirsi execution, model routing, permissions, schemas, or knowledge-root lookup.
