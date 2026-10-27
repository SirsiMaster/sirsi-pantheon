Memory liveness and Doctor alerts now label observed counters with their actual
UTC sampling window and GiB units. Liveness sizing copy distinguishes measured
MLX allocation from a model-ID weight estimate and states the applicable
KV-cache and OS-headroom formula. Session-candidate copy no longer asserts
Claude-session causality or directs reaping. Both surfaces require fresh
readings, exact owner/custody, and current-demand confirmation before supported
resolution at the workload's own idle boundary. Classification thresholds,
sizing formulas, admission, and runtime actions are unchanged.

Refs: `internal/liveness/livenesswatch.go`,
`internal/liveness/livenesswatch_test.go`, `internal/guard/deathspiral.go`,
`internal/guard/deathspiral_test.go`.
