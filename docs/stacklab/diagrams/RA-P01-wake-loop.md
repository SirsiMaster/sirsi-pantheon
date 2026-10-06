# RA-P01 — Wake loop (reactive inbox consumer)

Owner: Ra. Source: `internal/router/wake.go`, `consumer.go`, `consumerslots.go`. Revision: main `c1a78077`.
State labels: **implemented** = in source on main; **observed** = seen in a live log; **undeployed** = merged but not running (PR #639 gates).

## Logical view
```mermaid
flowchart TD
  A[LaunchAgent starts wake-loop for one lane] --> B[Register thread, read lane config from the pinned registry]
  B --> C{Consumer declared and usable?}
  C -- no, cwd or command invalid --> W[WATCH-ONLY: heartbeat only, never works the inbox]
  C -- yes --> D[Every interval: heartbeat and read inbox depth]
  D --> E{Work waiting?}
  E -- no --> D
  E -- yes --> G{All gates pass?}
  G -- no --> H[HELD: reason published in lane state] --> D
  G -- yes --> S[Spawn headless consumer: claude --print]
  S --> X[Consumer pulls, acknowledges, claims, closes with evidence]
  X --> M{Output matches a known failure?}
  M -- yes --> P[Publish the catalogued answer to the lane] --> D
  M -- no --> D
```
Gates, in order of evaluation: quarantine, idle CPU at least 10%, measurement window, no-progress back-off *(undeployed, #639)*, hourly spawn ceiling *(undeployed, #639)*, attended hold, host consumer cap (one per five cores; 2 on the M1, 3 on the M5).

## Data view
```mermaid
flowchart LR
  REG[(Pinned registry snapshot)] -->|consumer command, cwd, env| LOOP[wake-loop]
  LOOP -->|heartbeat, inbox depth, HoldAttended, HoldSlots| RELAY[Spool relay]
  RELAY -->|host token| SVC[Router service] --> DB[(Postgres ledger)]
  LOOP -->|argv, env, cwd| CONS[Headless consumer]
  CONS -->|pull, ack, claim, close and evidence| RELAY
  CONS -->|stdout and stderr| LOOP
  LOOP --> LOG[(~/.sirsi/logs/wake-lane.log)]
```
Trust boundary: the consumer never holds the host token; only the relay does. Paths in the registry are rebased onto the local home when another machine's home does not exist here.

## State view
```mermaid
stateDiagram-v2
  [*] --> Started
  Started --> WatchOnly: no usable consumer
  Started --> Wakeable: consumer resolved
  Wakeable --> Held: a gate refuses
  Held --> Wakeable: gate clears
  Wakeable --> Running: consumer spawned
  Running --> Wakeable: consumer exits
  Running --> Wakeable: stalled, terminated once and replaced
  WatchOnly --> [*]: launchd stops the job
```

## Failure and recovery
- Invalid cwd or missing command: WATCH-ONLY, lane reads WATCH_ONLY, nothing is spawned (by design: a consumer that cannot start must not read as armed).
- Consumer error or signal: logged, next cycle may spawn again, subject to the gates.
- Stalled consumer: terminated once and replaced (test `TestStalledConsumerIsTerminatedOnceAndReplaced`).
- Quarantine: no loop spawns anything until it is lifted. Recovery of an unknown failure class is by hand today (OPEN, see RA-P02).
