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
  Native host-cost qualification, golden-output acceptance on the actual
  target host, and a serialized multi-host rollout are explicit follow-ups —
  this change is source-only (build + focused race run + full
  `internal/router` suite green; no sampler run against a live production
  host as part of landing it).

Refs: PANTHEON_RULES.md §2.5/§2.13/§2.33 (A35), ROUTER_SERVICE_GOAL.md R7/G6
Changelog: internal/router/backpressure.go host-pressure probe
