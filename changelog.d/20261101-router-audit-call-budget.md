### Fixed
- Bounded the remote post-commit audit read by the existing router per-call timeout while preserving any earlier caller cancellation or deadline.

Refs: `docs/ADR-036-ROUTER-V2-DURABLE-DISPATCH.md`, `docs/ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md`, `internal/routerstore/remote_export.go`, `internal/routerstore/remote_test.go`
