### Fixed
- `sirsi router wake-latency` now measures first claim minus opened. It
  previously read `items.lease_updated`, which `RenewLease` and `StartWork`
  overwrite, so any started or renewed item reported wake latency plus working
  time. New column `items.first_claimed_at` (SQLite schema v27, Postgres schema
  version 27) is written once by `ClaimNext`/`ClaimItem` and never by renew,
  start-work or requeue. No backfill: items first claimed before v27 are
  excluded, as are never-claimed items. The JSON sample key is now
  `first_claimed_at` (was `lease_updated`). Apply the Postgres schema before
  deploying the router service.
- The shared `pct` percentile helper is now nearest-rank (`ceil(p*n/100)-1`).
  It previously used `int((n-1)*p/100)`, which understated the tail (p95 of
  four samples was the third smallest, not the largest). This also changes the
  p50/p95/p99 printed by `sirsi router bench report`, which shares the helper.

Refs: PANTHEON_RULES.md A35, ADR-063, task ra-m0-fabric-baseline
