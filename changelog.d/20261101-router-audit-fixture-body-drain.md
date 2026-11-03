### Fixed
- Drain and close the stalled remote `Get` fixture's POST body before waiting for request cancellation, and bound handler cleanup so cancellation regressions fail without hanging the test suite. Production behavior is unchanged.

Refs: `docs/ADR-036-ROUTER-V2-DURABLE-DISPATCH.md`, `internal/routerstore/remote_test.go`
