- Corrected the existing Ma'at binary-drift preview to resolve one exact System One case across the complete bounded journal, refusing duplicates and malformed retained tails, and stripped ANSI styling from the preview error label. Added inert retained-history regressions; native error presentation reuses the existing ANSI stripper and still requires installed-app verification. No updater or host action was run.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/maat/canon/COMPONENT_CATALOG.md`, `cmd/sirsi/maatrepair_binary_drift.go`, `internal/maat/decisions.go`
