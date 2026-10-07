# Design — Ra / Horus Fabric

The runnable predicate joins router item, durable ledger task, and canon
requirement. `routerstore.Store` is the backend contract; SQLite is for explicit
local tests/migration and PostgreSQL is the service backend. `dispatch.Facade`
is the only production write boundary.

Lease and wake mutations are durable and fenced. `ListenNotify`/`Wait` provide
edge-triggered delivery with a safety recheck; wake leases expire and become a
terminal failure after bounded attempts. `AckItem` records that the recipient
read the body; it never closes work. `Complete`/`Close` remain the completion
verbs.

Horus, CLI, MCP, TUI, menubar and dashboard are read models over the same
facts. A worker receipt is not completion until the requirement trace resolves.
No surface may infer a healthy lane from a PID or heartbeat alone.
