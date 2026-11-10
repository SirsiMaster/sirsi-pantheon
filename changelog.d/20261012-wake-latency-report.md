### Added
- `sirsi router wake-latency` — read-only M0 charter T4 instrument: opened ->
  first-claim (`lease_updated`) elapsed time per item, aggregated (p50/p95/max)
  or raw (`--json`). New `Store.WakeLatencySamples` method, implemented on
  `SQLiteStore`, wired through `RemoteStore` and `ruleOfRaExempt`.
  `lease_updated`/`opened` stay out of `Item`/`itemCols`
  (`TestFieldFidelityWithWorkItem`) — this is a separate measurement-only read
  path, not a struct field.

Refs: PANTHEON_RULES.md, task ra-m0-04-wake-latency-report (Ra charter M0, T4
slice of ra-m0-fabric-baseline)
