### Preserve measured context in routed Horus liveness findings

New liveness-watch alerts retain the existing remediation guidance and include
the measured finding detail, host, and UTC observation bracket. The appended
context is bounded and explicitly requires a fresh state check; it grants no
repair authority. Existing open alerts are not rewritten, and missing
historical observations cannot be reconstructed.

Refs: `PANTHEON_RULES.md` A27/A32, `docs/ADR-022-CTR-OS-TRUTH-LIVENESS.md`, `docs/ADR-040-DO-NO-HARM-TO-THE-RUNNING-HOST.md`, `internal/liveness/livenesswatch.go`.
