# RA-P16 — Mailbox alias and reassign (ADR-072 C5)

Owner: Ra. Source: `cmd/sirsi/routerreassigncmd.go`, `agents.json` `aliases` map, `cmd/sirsi/dashboard.go`/`dashboardrouter.go` alias filtering. Revision: main `b29778fc`. Shares its item-level mechanism with RA-P06 (`ReassignItem`) — this view is the retired-lane scenario, not a new primitive.

## Logical view
```mermaid
flowchart TD
  RETIRE[Lane retired — agents.json aliases: {old: successor}] --> NEWMAIL[send to old name]
  NEWMAIL --> AUTORESOLVE[New sends auto-resolve: delivered straight to successor]
  RETIRE --> BACKLOG{Open unclaimed items already sat in old's mailbox?}
  BACKLOG -- yes --> DRAIN[router drain-aliases: ReassignItem old to successor, id+history kept]
  BACKLOG -- no --> DONE1[nothing to drain]
  DRAIN --> DRYRUN{--dry-run?}
  DRYRUN -- yes --> REPORT[report only, no write]
  DRYRUN -- no --> DRAINED[items now in successor's mailbox]
  RETIRE --> BOARDS[dashboard / workboard: alias excluded from live-lane list]
```

## Data view
```mermaid
flowchart LR
  AGENTSJSON[(agents.json aliases map)] -->|Aliases()| RESOLVER[send-path resolver]
  RESOLVER -->|redirect to_agent| ROW[(items row, unchanged id)]
  AGENTSJSON -->|Aliases()| DASHFILTER[dashboard/workboard lane list]
  DRAINCMD[drain-aliases] -->|ReassignItem per open item| ROW
```

## Failure and recovery
- Items that arrived *before* retirement are not automatically redirected — they sit in the old mailbox until `drain-aliases` runs; a retirement that forgets this step silently strands a backlog that no live loop watches (the exact A27 failure mode this diagram set exists to catch).
- `--dry-run` exists so the drain can be verified before it mutates anything — same convention as every destructive-adjacent op (A1 Safety First).
- A retired name reappearing as a *new* alias target (name reuse) is governed separately by ADR-072 (naming-convention-enforcement P3 on this ledger, currently OPEN) — this diagram covers drain only, not reuse.
