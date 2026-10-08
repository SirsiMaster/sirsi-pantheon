# Component catalog — Ra / Horus

| Component | Source surface | Boundary | Release evidence/state |
|---|---|---|---|
| Canonical resolver | `internal/routerstore/resolve.go` | URL/service or explicit test DB | one-authority fix; verified locally |
| Dispatch facade | `internal/dispatch` | all production reads/writes | package tests; source-bound |
| Registry | `internal/router/registry.go` | identity and adapter metadata | origin/service drift doctor |
| Service | `cmd/sirsi router serve`, `internal/routerstore/serve.go` | Cloud Run/Cloud SQL | deployed historically; fresh readback open |
| Remote client | `internal/routerstore/remote.go` | session/nonce/runtime auth | contract tests |
| Relay | `cmd/sirsi/routerrelaycmd.go` | host token holder; no DB copy | M1/M5 host receipt |
| Leases | `tasklease.go`, `items.go` | fenced claim/renew/close | adversarial tests |
| Wake/informer substrate | `wake.go`, `wakeevents.go`, `horus` | bounded delivery, no blind spawn | shipped substrate; informer increment next |
| A2A/MCP | `cmd/sirsi-router-mcp` | read/mutate façade over dispatch | package tests |
| Horus | `internal/horus`, dashboard/fleet | derived observation only | source-bound/live per host |
| Stack Lab wing | `docs/stacklab/ra/canon` | docs and receipts | this release |
| Release/rollback | `scripts/release-train.sh`, recipe | immutable hash/readback | candidate contract |

Owner: Ra/Horus Fabric lane. No component may create a second authority.
