- Pinned retained binary-drift case fixtures before their filler records and pinned recent duplicate rows after the fillers. The regressions now assert exact evidence counts in `Recent(100)` before checking full-history uniqueness and refusal.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `cmd/sirsi/maatrepair_binary_drift_test.go`, `internal/maat/decisions.go`
