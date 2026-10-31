### Resolve an exact binary-drift finding after a fresh clear observation

Casebook now offers a separately confirmed, exact-host recheck for a single
canonical binary-drift finding. Ma'at records the bounded observation and
owner disposition in its existing journal, and resolves only that source case
as “no longer active”; it does not run or claim an updater. Mixed screens,
stale or foreign evidence, active or unavailable observations, and journal
errors remain open/refused. Repeated identical observations return the durable
existing disposition.

Diagnostic relief copy also distinguishes historical `Jetsam Events (7d)`
counts from current RAM, swap, and top-memory readings. Current readings make no
promise of immediate memory reclamation and direct the operator to recheck.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md`, `internal/maat/binary_drift_disposition.go`, `internal/maat/casebook/casebook.go`, `macapp/Sources/SirsiMenubar/MaatCasebookView.swift`, `macapp/Sources/SirsiMenubar/Views.swift`.
