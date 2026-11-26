### Fixed
- `sirsi router fabric` (and every `ListActive` caller) no longer falls back to the legacy full-ledger read when
  `SIRSI_ROUTER_URL` is unset on a cut-over host, as in a LaunchAgent with a minimal environment. `routerstore.Resolve`
  already recovered the service from `~/.sirsi/router-service.env`, but `routercfg.StoreWake` only looked at the
  environment and `~/.sirsi/store-wake`, so the store was the service while reads took the slow legacy path (8-10 s
  `ListActive` returning 5 items instead of 706, plus an empty thread registry). `StoreWake` now also treats
  `router-service.env` as the cut-over marker. Measured on the M1 with `env -i HOME PATH=/usr/bin:/bin:...`: see the PR.

- The Fabric snapshot stage now reads open items, tasks and the thread registry concurrently (same calls, same error
  precedence), so the stage costs the slowest read rather than the sum; the 15 s acquisition ceiling is unchanged.

Refs: PANTHEON_RULES.md A35, ADR-062 section 1
