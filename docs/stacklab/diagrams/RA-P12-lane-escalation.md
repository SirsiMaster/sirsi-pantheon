# RA-P12 — Lane escalation ("lane needs you" alert and auto-resolve)

Owner: Ra. Source: `internal/router/laneescalation.go`. Revision: main `b29778fc`.

## Logical view
```mermaid
flowchart TD
  PASS[Supervisor pass, 60s cadence] --> JUDGE[supervision produces Escalations: lane unreachable / beyond auto-repair]
  JUDGE --> ROUTE[RouteLaneEscalations]
  ROUTE --> OPEN[Open owner's dispatch inbox]
  OPEN -->|read fails| FAILCLOSED[fail closed — never duplicate escalations]
  OPEN -->|ok| TITLES[Build openTitles set of currently-open cards]
  TITLES --> DEDUP{escalation title already open?}
  DEDUP -->|yes| SKIP[skip — no re-mint]
  DEDUP -->|no| SEND[f.Send horus to owner: Lane needs you]
  SEND -->|error| CONTINUE[record first error, keep sending the rest]
  ROUTE --> RESOLVE[resolveClearedAlerts]
  RESOLVE --> SCAN[scan open 'Lane needs you' cards sent by horus]
  SCAN --> CLEARED{lane no longer in current escalation set?}
  CLEARED -->|yes| CLOSE[CloseItem, auto-resolved note]
  CLEARED -->|no| LEAVEOPEN[leave open]
```

## Data view
```mermaid
flowchart LR
  SUPERVISOR[supervisor pass] -->|Escalation list| ROUTER[RouteLaneEscalations]
  ROUTER -->|Send horus->owner| INBOX[(owner dispatch inbox)]
  INBOX -->|open items, title match| DEDUPSET[openTitles set]
  ROUTER -->|CloseItem| INBOX
```

## Failure and recovery
- A dispatch-inbox read failure fails the whole pass closed rather than sending — duplicating the same "lane needs you" card on every retry would be worse than a missed escalation this cycle.
- Dedup is by exact title match against currently-open cards, so a lane that stays down across passes gets one card, not one per pass.
- One send failure does not stop the pass — the remaining escalations still get sent; only the first error is recorded.
- `resolveClearedAlerts` closes a card with an auto-resolved note the moment its lane drops out of the current escalation set — closure errors are ignored on purpose, since a stale card left open just reverts to the pre-auto-resolve behavior, never a reason to abort the pass.
