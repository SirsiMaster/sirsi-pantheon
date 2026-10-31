### Fixed
- Retain worker retirement keys while clearing retired payloads, so compatible stale writers cannot resurrect pruned identities. Exact lane, host and observation predicates remain enforced; normal readers hide fences and unrelated fresh registrations remain available.
- Exercise both independently seeded fences and the real pruning producer, including stale-write and retired-ID refusals, opportunistic pruning during registration, and preserved ownership, timestamps and empty payloads for all three retained keys.
- Keep canonical service adoption separate: Ra/SSA must verify the compatible SQL-writer and rollback floor before activation. Source qualification does not claim live pruning or deployment.

Refs: ADR-057, ADR-062, internal/routerstore/README.md
