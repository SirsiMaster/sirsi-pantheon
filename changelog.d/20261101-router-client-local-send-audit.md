### Fixed
- Remote pre-cutover sends now fetch the committed item through authenticated `Get` and write the canonical markdown audit projection on the client, without sending a client filesystem path to the router service. Projection failures preserve the committed result and do not retry dispatch; local SQLite export and StoreWake behavior are unchanged.

Refs: `docs/ADR-036-ROUTER-V2-DURABLE-DISPATCH.md`, `docs/ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md`, `internal/dispatch/facade.go`, `internal/routerstore/export.go`
