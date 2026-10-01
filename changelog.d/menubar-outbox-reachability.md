- feat(menubar): decode node-status's `outbox[]` (ADR-069, PR #931) into
  `RBOutbox` and surface an unreadable relay outbox as a blocker
  (`OutboxBlockerCard`) — a decoder that modeled the field but never reached a
  view would reintroduce, in Swift, the exact false-quiet-zero bug PR #931
  fixed on the Go side.
- test(menubar): `OutboxReachabilityTests` — the new `macapp` test target
  proving the decode actually reaches `routerHasBlockers`, not just that it
  parses.
