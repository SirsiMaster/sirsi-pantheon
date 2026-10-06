# RA-P03 — Origin-pinned agent registry (A37)

Owner: Ra. Source: `internal/router/registrysnapshot.go`, `cmd/sirsi/routerregistrycmd.go`. Revision: main `c1a78077`.

## Logical view
```mermaid
flowchart TD
  S[router registry sync] --> G[git show origin/main:.agents/idea-router/agents.json]
  G --> W[Write snapshot under ~/.sirsi/registry, scoped to this router root]
  W --> P[Registry reads prefer the snapshot while it is fresh]
  P --> Q{Snapshot older than 72 hours?}
  Q -- yes --> I[Snapshot ignored - reads fall back to the working tree]
  Q -- no --> P
  P --> E[Edits to the working-tree registry are refused while pinned]
  E --> U[router registry unpin removes the pin]
  S --> H[sync --install schedules an hourly launchd job]
  H --> S
```
## Data view
```mermaid
flowchart LR
  ORIGIN[(origin/main agents.json)] -->|git show| SNAP[(~/.sirsi/registry snapshot)]
  SNAP -->|lane config| LOOPS[wake loops, ping, dispatch]
  WT[(working-tree agents.json)] -. ignored while pinned .-> LOOPS
```
Observed 2026-10-05: lanes read WATCH_ONLY because registry paths were another machine's home; fixed by path rebase in the resolver (#982), not by editing the pinned file.

## Failure and recovery
- git or origin unreachable: the existing fresh snapshot keeps serving, up to 72 hours.
- Stale or missing snapshot: falls back to the working tree. Whether that fallback should instead fail closed is OPEN.
