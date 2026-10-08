# ADR-076 — One router surface, three front doors

- **Status:** Proposed — owner-directed implementation, 2026-10-08
- **Steward:** `ra` (router service), with Pantheon as the surface owner
- **Scope:** read-only operator and embedded frontend surface; no new router authority
- **Refs:** ADR-062, ADR-068, ADR-069, ADR-072

## Decision

The router has one browser read model and three supported front doors:

1. **Standalone:** Pantheon serves `/router` from `sirsi board-serve`.
2. **Pantheon/Horus:** the existing dashboard links to and may consume the same
   versioned surface contract.
3. **Nexus:** the portal discovers and embeds `/router?surface=nexus`, or consumes
   the contract directly when embedding is unavailable.

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

`GET /api/router/v1/snapshot` returns the last successful producer snapshot.
Before the first successful poll it returns `503`, not fabricated zero state.
The stream and projections are no-store responses. Cross-origin reads use an
explicit allowlist; wildcard credentialed CORS is not permitted.

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
