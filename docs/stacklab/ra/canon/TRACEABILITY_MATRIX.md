# Traceability matrix — Ra / Horus

| Requirement | Source | Evidence in this release | State |
|---|---|---|---|
| One production authority | `routerstore.Resolve`, `dispatch.Open` | explicit-local-fallback negative control | verified locally |
| Runnable predicate joins three sources | `internal/routerstore/runnable.go` | package tests | source-bound |
| Claims are fenced/idempotent | `tasklease.go`, `facade.go` | lease/expiry/duplicate tests | source-bound |
| One physical Studio capacity pool | shared scheduler alias comparisons and conservative capacity reads | both-direction/history/concurrent alias fixtures; ADR-118 | source reviewed; canonical service/native client readback pending |
| Wake transitions are durable | `wakeevents.go`, `reconcile.go` | event synthesis/terminal retry tests | source-bound |
| A2A identity/body/correlation/errors | `dispatch`, `routerstore` | contract assessment and tests | source-bound |
| Read acknowledgement is distinct | `AckItem`, schema v22+ | recipient-only ack tests | source-bound |
| Cloud canonical service | `router serve`, GCP recipe | historical deploy receipts; current readback pending permissions | open-live |
| Codex least-privilege delivery | `router relay`, spool contract | M1/M5 relay evidence | host-verified prior receipt |
| Restricted-worker native execution | `scripts/maat/fabric-run.sh`, exact-IP proxy profiles | all declared Codex profile guards, POSIX argv/refusal tests; scoped SSA M1/Studio success; external-domain denial | source/transport verified; each lane still needs its own actual destination receipts |
| M1/M5 fleet parity | relay/client/service | current fresh cross-host proof | partial |
| Fresh third-machine join | token/env/register/claim/close | hermetic rehearsal script | rehearsal pending |
| Horus/CLI/MCP same read model | `routerboard`, `router-mcp`, Horus | source + package tests | source-bound |
| Rollback/revocation | deploy/cutover scripts | rollback fixture; fresh cloud rehearsal pending | partial |
| Commercial product closure | commercialization gate | pilot/platform-foundation classification | open |
