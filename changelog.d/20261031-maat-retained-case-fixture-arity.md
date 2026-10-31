- Corrected the retained binary-drift case regression fixture to capture all four values returned by its existing setup helper. Production behavior and the missing-case, host, digest, and duplicate-evidence assertions are unchanged.

Refs: `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `cmd/sirsi/maatrepair_binary_drift_test.go`
