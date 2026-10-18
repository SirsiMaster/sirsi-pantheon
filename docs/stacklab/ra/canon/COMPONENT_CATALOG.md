# Component catalog — Ra / Horus

| Component | Source surface | Boundary | Release evidence/state |
|---|---|---|---|
| Canonical resolver | `internal/routerstore/resolve.go` | URL/service or explicit test DB | one-authority fix; verified locally |
| Dispatch facade | `internal/dispatch` | all production reads/writes | package tests; source-bound |
| Registry | `internal/router/registry.go` | identity and adapter metadata | origin/service drift doctor |
| Service | `cmd/sirsi router serve`, `internal/routerstore/serve.go` | Cloud Run/Cloud SQL | deployed historically; fresh readback open |
| Remote client | `internal/routerstore/remote.go` | session/nonce/runtime auth | contract tests |
| Relay | `cmd/sirsi/routerrelaycmd.go` | host token holder; no DB copy | M1/M5 host receipt |
| Scoped native transport | `scripts/maat/fabric-run.sh`, registry Codex consumer profiles | exact enrolled IP proxy, pinned SSH, no bearer export; both ends supervised | literal-argv/refusal fixtures; actual scoped SSA M1/Studio terminals2311a0d6/d3f5f7aa; per-lane adoption remains tracked |
| Leases | `tasklease.go`, `items.go` | fenced claim/renew/close | adversarial tests |
| Studio capacity aliases | `internal/maat/schedule/resource.go`, shared atomic scheduler | bare legacy/canonical names share capacity; original IDs/spellings and host binding retained | independent source inspection/native race fixtures; actual deployment still required |
| Wake/informer substrate | `wake.go`, `wakeevents.go`, `horus` | bounded delivery, no blind spawn | shipped substrate; informer increment next |
| A2A/MCP | `cmd/sirsi-router-mcp` | read/mutate façade over dispatch | package tests |
| Horus | `internal/horus`, dashboard/fleet | derived observation only | source-bound/live per host |
| Stack Lab wing | `docs/stacklab/ra/canon` | docs and receipts | this release |
| Release/rollback | `scripts/release-train.sh`, recipe | immutable hash/readback | candidate contract |

Owner: Ra/Horus Fabric lane. No component may create a second authority.
