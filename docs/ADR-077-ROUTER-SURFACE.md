# ADR-077 — One router surface, three front doors

- **Status:** Proposed — owner-directed implementation, 2026-10-08
- **Steward:** `ra` (router service), with Pantheon as the surface owner
- **Scope:** read-only operator and embedded frontend surface; no new router authority
- **Refs:** ADR-062, ADR-068, ADR-069, ADR-072

## Decision

The router has one browser read model and three supported front doors:

1. **Standalone:** the supervised Pantheon/Horus server serves `/router`.
2. **Pantheon/Horus:** the existing dashboard and the standalone page call the
   same typed `RouterFn` producer.
3. **Nexus:** the portal discovers and consumes the versioned snapshot contract.

All three are projections of the Ra-owned router service. They do not open
SQLite, read `agents.json`, reconstruct leases, or maintain local copies of
router state. A surface that cannot reach the canonical producer reports
unavailable; it never renders an empty or stale board as current truth.

## Contract

Discovery is `GET /api/router/v1/manifest` and must identify:

- schema `router-surface.v1`;
- authority `ra`;
- producer build identity;
- the canonical snapshot, stream, ledger, and task endpoints; and
- the declared Pantheon/Nexus integrations.

`GET /api/router/v1/snapshot` returns the current producer snapshot or `503`.
It never fabricates zero state. Cross-origin reads use an explicit allowlist;
wildcard credentialed CORS is not permitted.

The standalone page keeps mutations separate from the read contract. Existing
operator actions remain behind the established router controls and are not
made available through the embedded Nexus frame.

## Consequences

- The independent surface can be opened without Nexus or a native dashboard.
- Nexus and Pantheon show the same producer-backed state instead of competing
  local boards.
- A service outage is visible as an outage, making stale-data incidents
  diagnosable rather than silently survivable.
- A future remote or hosted frontend may consume the versioned contract without
  changing the router store or identity model.

This ADR does not claim production deployment, remote-host transport, or
authentication changes. Those remain separate release work.
