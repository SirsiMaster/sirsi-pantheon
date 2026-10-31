### Preserve measured context in routed Horus liveness findings

New liveness-watch alerts retain the existing remediation guidance and include
the measured finding detail, host, and UTC observation bracket. Detail content
is capped at 2048 UTF-8 bytes, with a truncation marker appended outside that
content cap. The added context explicitly requires a fresh state check and
grants no repair authority. Existing open alerts are not rewritten, and missing
historical observations cannot be reconstructed.

Refs: `PANTHEON_RULES.md` A27/A32, `docs/ADR-022-CTR-OS-TRUTH-LIVENESS.md`, `docs/ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md`, `internal/liveness/livenesswatch.go`.

### Resolve an exact binary-drift finding after a fresh clear observation

Casebook now offers a separately confirmed, exact-host recheck for a single
canonical binary-drift finding. Ma'at records the bounded observation and
owner disposition in its existing journal, and resolves only that source case
as “no longer active”; it does not run or claim an updater. Mixed screens,
stale or foreign evidence, active or unavailable observations, and journal
errors remain open/refused. Repeated identical observations return the durable
existing disposition.

### Describe relief from the finding's actual time scope

Diagnostic relief copy now distinguishes the historical `Jetsam Events (7d)`
count from current RAM, swap, and top-memory readings. Current readings make no
promise of immediate memory reclamation and direct the operator to recheck.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/maat/canon/TRACEABILITY_MATRIX.md`, `internal/maat/binary_drift_disposition.go`, `internal/maat/casebook/casebook.go`, `macapp/Sources/SirsiMenubar/MaatCasebookView.swift`, `macapp/Sources/SirsiMenubar/Views.swift`.
