- Tightened the retained binary-drift case lookup to count every exact evidence-reference match across kinds before checking the sole row's System One type. Added an old/new-window regression for a different-kind duplicate; the existing kind-specific lookup remains unchanged.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/maat/canon/COMPONENT_CATALOG.md`, `internal/maat/decisions.go`, `cmd/sirsi/maatrepair_binary_drift.go`
