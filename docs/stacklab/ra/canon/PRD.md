# PRD — Ra / Horus Fabric

Provide one durable, inspectable control plane for workers, tasks, leases, wake
events, host relays, and node status. A dispatch must be attributable,
idempotent, recoverable, and visible to the intended recipient without requiring
that recipient to maintain a second authority.

### Product outcomes

- one canonical ledger across M1, M5, Claude, Codex and future hosts;
- safe A2A semantics over the existing service and relay, including read ack;
- truthful Horus/CLI/MCP views from the same store;
- bounded recovery from stale sessions, failed wakes, service outages and client
  drift;
- a reproducible Stack Lab recipe that an operator can execute and audit.

Ra does not own another lane's credentials, source, router database, customer
data, or production deployment. It owns the routing mechanism and its evidence.
