### Fixed

The wake-loop terminal observer regression now synchronizes on the existing post-heartbeat depth-transition log. The fixture reads back the authoritative terminal state before allowing the next heartbeat, then verifies `ErrHeartbeatThreadUnavailable` and no revival. Production lifecycle retry limits and heartbeat behavior are unchanged.

Refs: `docs/ADR-061-DURABLE-CONTINUOUS-EXECUTION-ENFORCEMENT.md`, `docs/stacklab/ra/canon/TRACEABILITY_MATRIX.md`, `contracts/stacklab/ra-horus-fabric-recipe-v1.json`
