# Traceability matrix — Ra / Horus

| Requirement | Source | Evidence in this release | State |
|---|---|---|---|
| One production authority | `routerstore.Resolve`, `dispatch.Open` | explicit-local-fallback negative control | verified locally |
| Runnable predicate joins three sources | `internal/routerstore/runnable.go` | package tests | source-bound |
| Claims are fenced/idempotent | `tasklease.go`, `facade.go` | lease/expiry/duplicate tests | source-bound |
| Wake transitions are durable | `wakeevents.go`, `reconcile.go` | event synthesis/terminal retry tests | source-bound |
| A2A identity/body/correlation/errors | `dispatch`, `routerstore` | contract assessment and tests | source-bound |
| Read acknowledgement is distinct | `AckItem`, schema v22+ | recipient-only ack tests | source-bound |
| Cloud canonical service | `router serve`, GCP recipe | historical deploy receipts; current readback pending permissions | open-live |
| Codex least-privilege delivery | `router relay`, spool contract | M1/M5 relay evidence | host-verified prior receipt |
| M1/M5 fleet parity | relay/client/service | current fresh cross-host proof | partial |
| Fresh third-machine join | token/env/register/claim/close | hermetic rehearsal script | rehearsal pending |
| Horus/CLI/MCP same read model | `routerboard`, `router-mcp`, Horus | source + package tests | source-bound |
| Rollback/revocation | deploy/cutover scripts | rollback fixture; fresh cloud rehearsal pending | partial |
| Commercial product closure | commercialization gate | pilot/platform-foundation classification | open |
