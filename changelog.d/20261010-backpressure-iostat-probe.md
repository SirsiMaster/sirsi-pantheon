- Router host-pressure probe: replace `top -l 2` process-enumerating sample
  with aggregate `iostat -d -C -n 0 -c 2 -w 1` CPU counters (R7/G6,
  `internal/router/backpressure.go`). `top`'s process-table scan was itself a
  source of load under contention; `iostat`'s two-row CPU-aggregate sample
  avoids enumerating processes at all, with a `sysctl -n vm.loadavg` fallback
  preserved unchanged for when `iostat` is unreadable. Adds cross-process
  coordination (`coordinatedHostLoad`, `flock`-serialized, one probe per host
  per 30s TTL, never unlinking the lock inode) and a typed unknown-result
  cache (v2 cache record format) that amortizes a failed probe for the same
  TTL instead of retrying on every call — a failed sampler must not itself
  become a source of repeated host contention. Existing `backpressure_test.go`
  dispatch-admission tests are unchanged; `backpressure_cache_test.go` is
  migrated to exercise real expired on-disk cache records instead of a
  mutable clock seam; `backpressure_interval_test.go` and
  `coordination_test.go` are new, covering the iostat row-parsing contract
  and cross-process/crash/contention locking behavior respectively.
  Accepted tradeoffs (full detail in
  `docs/ROUTER_HOST_PRESSURE_UNKNOWN_POLICY.md`): a cache-write failure
  permits sequential retries rather than guaranteeing the one-probe-per-TTL
  cost bound; an un-upgraded binary still on the old `top` sampler does not
  participate in the shared lock/cache and gets no cross-process
  coordination until it's upgraded; `iostat -w 1`'s one-second interval is
  nominal, not guaranteed (the OS can return early), with no fixed numeric
  temporal-error bound established; and no native sampler cost claim is
  made for any production host. Native host-cost qualification,
  golden-output acceptance on the actual target host, and a serialized
  multi-host rollout are explicit follow-ups — this change is source-only
  (build + focused race run + full `internal/router` suite green; no
  sampler run against a live production host as part of landing it).
  A CI-only regression (job 114116632633, caught in independent review of
  the first head) is fixed in the same PR before merge consideration: a
  fresh home with no `~/.sirsi` yet failed to create the cache's lock file
  (missing parent directory) and silently returned unknown without ever
  probing — `coordinatedHostLoad` now creates that directory up front, with
  a regression test (`TestCoordinatedHostLoadCreatesColdHomeCacheDir`)
  reproducing the cold-home condition without a live sampler.

Refs: PANTHEON_RULES.md §2.5/§2.13/§2.33 (A35), ROUTER_SERVICE_GOAL.md R7/G6
Changelog: internal/router/backpressure.go host-pressure probe
