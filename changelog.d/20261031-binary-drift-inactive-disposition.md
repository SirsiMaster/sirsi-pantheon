### Resolve an exact binary-drift finding after a fresh clear observation

Casebook now offers a separately confirmed, exact-host recheck for a single
canonical binary-drift finding. Ma'at records the bounded observation and
owner disposition in its existing journal, and resolves only that source case
as “no longer active”; it does not run or claim an updater. Mixed screens,
stale or foreign evidence, active or unavailable observations, and journal
errors remain open/refused. Repeated identical observations return the durable
existing disposition.

Paged Casebook history validates the disposition against the unique source
screen and host before status filtering or page projection. The query identity
changes so older continuations are refused and can be refreshed. Confirmation
checks cancellation before and after its fresh scan. The existing three-second
ceiling applies to that observation scan; it does not bound subsequent strict
journal validation or append.

Paged history carries the frozen System One source count and exact all-kind
evidence count into projection. A rejected typed disposition leaves verified
repair outcomes and pending attempts visible. Inactive rechecks retain their
own progress description across detail-view replacement and do not show
updater timing.

Diagnostic relief copy also distinguishes historical `Jetsam Events (7d)`
counts from current RAM, swap, and top-memory readings. Current readings make no
promise of immediate memory reclamation and direct the operator to recheck.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md`, `internal/maat/binary_drift_disposition.go`, `internal/maat/history_query.go`, `internal/maat/history_prefix_unix.go`, `internal/maat/history_prefix_windows.go`, `internal/maat/history_prefix_test.go`, `internal/maat/history_query_test.go`, `internal/maat/history_repair_query_test.go`, `internal/maat/casebook/casebook.go`, `internal/maat/casebook/casebook_test.go`, `cmd/sirsi/maatrepair_binary_drift_inactive.go`, `cmd/sirsi/maatrepair_binary_drift_inactive_test.go`, `macapp/Sources/SirsiMenubar/MaatCasebookView.swift`, `macapp/Sources/SirsiMenubar/MaatOperationState.swift`, `macapp/Tests/SirsiMenubarTests/MaatOperationStateTests.swift`, `macapp/Sources/SirsiMenubar/Views.swift`.
