# Traceability matrix — Ra / Horus

| Requirement | Source | Verification/evidence | State |
|---|---|---|---|
| Runnable predicate joins three sources | `internal/routerstore/runnable.go` | router/ledger/canon fixtures | source-bound |
| Claims are fenced and idempotent | `internal/routerstore/tasklease.go` | lease expiry and duplicate-claim tests | source-bound |
| Wake transitions are durable | `internal/routerstore/wakeevents.go` | event synthesis tests | source-bound |
| State is store-derived | `internal/routerstore/lanestate.go` | state transition tests | source-bound |
| Providers conform | `internal/router/registry.go` and adapters | adapter conformance receipts | OPEN for external peers |
| Installed recovery is autonomous | setup and Horus launch paths | restart/crash/stall host evidence | OPEN |
