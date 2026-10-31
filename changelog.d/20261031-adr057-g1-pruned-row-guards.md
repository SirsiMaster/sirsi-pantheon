### ADR-057 G1 compatible thread-store guards

- Preserve existing internal `pruned` rows across compatible upserts and
  physical deletes, reject callers that try to manufacture the reserved status,
  and hide retained rows from ordinary reads while keeping authority and raw
  migration visibility intact. G1 creates no fences and does not close the
  absent-row stale-insert residual; no prune activation or G2 behavior is added.

Refs: `docs/ADR-057-OPERATIONAL-ENFORCEMENT-SUPERVISION.md`, `internal/routerstore/threads.go`
