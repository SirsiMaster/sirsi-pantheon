### Fixed

- **There is exactly one router store: no implicit local ledger.** `routerstore.LocalPath()` no longer defaults to `~/.sirsi/router.db`. With neither `SIRSI_ROUTER_URL` nor an explicit `SIRSI_ROUTER_DB`, the router now refuses ("no router configured") instead of opening a second, silent store. On 2026-10-07 thirty items written by SSA's M5 session through a stale dev binary landed in that retired file and never reached the service or their recipients (ten were addressed to Ra).
