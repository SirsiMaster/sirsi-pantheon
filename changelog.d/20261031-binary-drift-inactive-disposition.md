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
changes so older continuations are refused and can be refreshed. The
confirmation scan keeps the existing three-second observation ceiling;
cancellation or deadline expiry does not append a disposition.

Paged history candidate materialization now carries the frozen System One
source count from its selected link reference, matching the bounded history
projection contract.

Diagnostic relief copy also distinguishes historical `Jetsam Events (7d)`
counts from current RAM, swap, and top-memory readings. Current readings make no
promise of immediate memory reclamation and direct the operator to recheck.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md`, `internal/maat/binary_drift_disposition.go`, `internal/maat/history_prefix_unix.go`, `internal/maat/history_prefix_test.go`, `internal/maat/casebook/casebook.go`, `macapp/Sources/SirsiMenubar/MaatCasebookView.swift`, `macapp/Sources/SirsiMenubar/Views.swift`.
