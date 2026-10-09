# RA-P08 — Spool relay forwarding

Owner: Ra. Source: `internal/routerstore/spool.go`, `cmd/sirsi/routerrelaycmd.go`. Revision: main `b29778fc`.

## Logical view
```mermaid
flowchart TD
  CALL[Lane call, SIRSI_ROUTER_URL=spool://dir] --> SLOT[acquireSlotIn: one of 64 in-flight slots]
  SLOT --> WRITE[Write req/ID.json atomically, tmp+rename]
  WRITE --> POLL[Lane polls res/ID.json, 30s wait]
  RELAY[sirsi router relay serve, 150ms poll] --> DISCOVER[discover pending req files]
  DISCOVER --> CONSUME[rename into inflight/]
  CONSUME --> FORWARD[forward: swap Authorization for host token, POST to service]
  FORWARD -->|ok| RESPOND[write res/ID.json]
  RESPOND --> POLL
  FORWARD -->|unreachable| HOLD[statusHoldForRetry]
  HOLD --> OUTBOX[move to outbox/, reply QUEUED_FOR_RETRY]
  OUTBOX --> DRAIN[drainOutbox on later wake, strict id order]
  DRAIN -->|still unreachable| OUTBOX
  DRAIN -->|ok| RESPOND
```

## Data view
```mermaid
flowchart LR
  LANE[lane process] -->|signed request| REQ[(spool/agent/req/ID.json)]
  REQ -->|consume| INFLIGHT[(spool/agent/inflight/ID.json)]
  INFLIGHT -->|forward success| SVC[(router service)]
  INFLIGHT -->|forward fails| OUTBOXQ[(spool/agent/outbox/ID.json)]
  SVC --> RES[(spool/agent/res/ID.json)]
  OUTBOXQ -->|retried in order| SVC
  INFLIGHT -->|post-send failure| FAILEDQ[(spool/agent/failed/ID.json, audit only)]
```

## Failure and recovery
- The lane's 30s wait (`newSpoolTransport`) distinguishes "never picked up" (safe to retry) from "OUTCOME UNKNOWN — consumed but no response" (never auto-retried if it was a mutation) — the ambiguity is surfaced, not papered over.
- `spoolRefused` hard-blocks `MintHostToken`/`RevokeHostToken`/`ListHostTokens` from ever crossing the relay by name, regardless of caller.
- Unreachable-service forwards land in `outbox/` and drain in strict id order on a later wake (`outboxRetryBackoff = 30s`), stopping at the first still-unreachable item — an ordered-release guarantee, not a race of whichever retries first.
- A failure discovered only after the request was already sent is parked in `failed/` for audit — never silently retried (would double-apply a mutation) and never silently dropped (would lose it).
