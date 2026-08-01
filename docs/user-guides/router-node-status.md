# `sirsi router node-status` — Horus operator view

**One-line:** show what's actually alive on this workstation — agents, threads,
queue depth, dispatch failures, drift, auth — in one read-only command.

This is the operator surface for the Horus ops-dashboard (ADR-026). It is the
CLI mirror of `GET /api/node-status`; the menubar's compact row and the TUI's
ops pane render the same read-model.

## Usage

```sh
sirsi router node-status            # styled human render (default)
sirsi router node-status --json     # raw JSON (contract-identical to the HTTP endpoint)
```

The verb is **read-only**: it never registers a thread, writes the inbox, or
mutates registry state. Safe to run from any context, including audit scripts.

## What you'll see

```text
𓂀  Horus Node Status   (schema 1.0.0)

  Repo:        /Users/.../sirsi-pantheon
  Router home: /Users/.../sirsi-pantheon/.agents/idea-router

  Agents:           19 registered
  Queue:            0 pending across 0 agents (41 completed topics)
  Live threads:     12 (stale: 4)
  Recent failures:  5 (newest first)
  Daemon:           ai.sirsi.idea-router — ok (/opt/homebrew/bin/sirsi)

  Live threads:
    • thr-abdb…  agent=claude-deck       surface=claude    pid=3981   os=alive    idle=3s
    • thr-9451…  agent=claude-pantheon   surface=claude    pid=96124  os=alive    idle=18s
    …
  Stale threads:
    • thr-78a6…  agent=claude-home  pid=6138   idle=429s
    …
  Pending by agent:
    claude-pantheon: 3
  Agent CLI health:
    claude: ok
    codex:  needs-login — not authenticated (blocking 2 items)
```

## Reading the output

- **Live threads** — registered threads whose anchor PID is alive (OS-truth per
  ADR-022). A `os=defunct` or `os=gone` line means the reaper will collect it
  on the next pass.
- **Stale threads** — alive PID but no recent heartbeat — surface a watcher
  that armed but isn't ticking (an A27 violation).
- **Recent failures** — last 5 work-queue dispatch failures, newest first.
- **Daemon** — `ok` / `go-run` / `configured-binary-missing` (the ADR-023
  binary-drift class surfaces here automatically).
- **Agent CLI health** — `needs-login` means the CLI is installed but
  unauthenticated; the count in parentheses is how many inbox items are
  blocked behind that auth failure.

## Flags

| Flag | Default | Description |
| :--- | :--- | :--- |
| `--json` | `false` | Output raw JSON. The shape is **contract-identical** to `GET /api/node-status` (same schema; the CLI pretty-prints with 2-space indent, the HTTP body uses compact encoding). Safe to pipe to `jq`. |

## JSON shape

The output carries `schema_version` (currently `"1.0.0"`). The shape is
additive — surfaces should decode tolerantly and ignore unknown fields. The
version bumps only on a breaking change (rename or type change); new fields
do not bump.

Top-level fields: `schema_version`, `router_home`, `repo_root`,
`registered_agents`, `agent_count`, `pending_by_agent`, `total_pending`,
`active_topics`, `completed_count`, `work_item_summary`, `daemon_installed`,
`daemon_loaded`, `daemon_label`, `configured_binary`, `binary_exists`,
`binary_is_go_run`, `last_claude_read`, `last_codex_read`, `agent_health`,
`wake_health`, `recent_failures`, `live_threads`, `stale_threads`,
`live_thread_count`.

## HTTP endpoint (same read-model, two transports)

```sh
curl -s http://127.0.0.1:7531/api/node-status                # full NodeStatus
curl -s 'http://127.0.0.1:7531/api/node-status?view=summary' # bounded OpsSummary (menubar)
```

The `summary` view returns a smaller `OpsSummary` shape with:
- `live_thread_count` / `stale_thread_count` / `queue_open_items` /
  `recent_failure_count` / `suspended_threads` — pure rollups.
- `has_drift_or_auth_issue` + `worst_icon` (🟢/🟡/🔴) for the menubar's lead
  row.
- `agents[]` — bounded to 12 rows (top-N by pending+live signal), with
  `more_agents` carrying the count of agents not shown.

The summary is a **pure reduction** of the same NodeStatus — every field is
derived; nothing is sourced independently. Counts reconcile exactly between
the full and summary views.

## Related

- **`sirsi router status`** — queue-only summary (older, narrower; this verb
  supersedes it for the operator view).
- **`sirsi thread list`** — read-only raw thread registry dump, including each
  thread's repository and workstream identity (debug-level; node-status
  is the curated view).
- **ADR-026** — the design (`docs/ADR-026-HORUS-OPS-DASHBOARD.md`).
- **R4 inventory** — what the read-model aggregates
  (`docs/HORUS_OPS_READMODEL_R4_INVENTORY.md`).
