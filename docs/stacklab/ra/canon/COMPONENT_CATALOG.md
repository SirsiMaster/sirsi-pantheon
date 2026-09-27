# Component catalog — Ra / Horus

| Component | Source surface | Write boundary | Evidence / next action |
|---|---|---|---|
| Registry | `internal/router/registry.go` | registry-owned records | external peer mappings |
| Runnable predicate | `internal/routerstore/runnable.go` | requirement evaluation | conformance fixtures |
| Leases | `internal/routerstore/tasklease.go` | fenced claim/renew/close | duplicate and expiry tests |
| Wake | `internal/routerstore/wakeevents.go` | durable event append | transition coverage |
| Horus state | `internal/routerstore/lanestate.go` | derived read model | restart evidence |
| Completion | `internal/routerstore/completion.go` | proof-backed close | requirement trace receipts |

Owner: Ra/Horus Fabric lane. Release state: source and registry contracts are merged; external peer and installed-runtime evidence remain open.
