# Ra Router Architecture — canonical plan

**Status:** release-candidate plan, 2026-10-07
**Authority:** `sirsi-pantheon` source and the GCP router service; this file is the
Stack Lab explanation, not a second registry or database.
**Scope:** Ra router, Horus observation, A2A/MCP access, host relays, and the
operational recipe that keeps them on one authority.

## Decision

Ra is a durable control plane with one ledger and several bounded surfaces:

```text
agent / CLI / MCP / Horus
          |
    dispatch.Facade                 one write/read boundary
          |
 routerstore.Resolve()              URL -> explicit test DB only
          |
   RemoteStore -> router serve -> Cloud SQL PostgreSQL
          |                  |
     host relay             leases, identity, wake events, audit
          |
     isolated lanes (Codex/Claude)
```

The local SQLite ledger is an explicit test and migration backend. It is not a
production fallback. A missing service configuration fails closed; it must never
create or reopen `~/.sirsi/router.db`. The retained historical local store is an
archive only and is not to be deleted or reintroduced as a source.

## Roles: Ra and Horus (owner, 2026-10-23/24)

The owner set the split between the two services:

- **Ra is the router at the cloud or third-party machine level.** It is the
  `router serve` service and its single ledger. Every host, agent and surface
  reaches it upward; it is the only authority for items, tasks, leases and
  identity.
- **Horus is the local router and machine observer on every node.** It is an
  independent Pantheon service, not an IDE agent and not an inbox lane. On each
  Mac it (a) observes the machine (status, hygiene, processes, relay and
  outbox health) and reports to Ra, and (b) is the node's local routing edge:
  local traffic goes through the host relay and spool to Ra. Horus never holds
  a second ledger. Its local queue is the relay outbox, a transport buffer that
  drains in order to Ra, not a store of record (ADR-069).
- **Mail addressed to Horus** goes to its custodian through `mail_to` (Horus →
  Ra), because a service does not read an inbox (#1194).
- **Independent modules.** Ra (router service, relay, CLI verbs) and Horus are
  separately versioned components. Each can be updated without updating
  Mercury or the other Pantheon modules, and the reverse. Ra exposes both a
  standalone front end and integrated views (fabric and ledger boards) inside
  the Pantheon surfaces, all held to the Swift Mac UI/UX standard.

**Status, stated honestly.** The roles above are the decided architecture. The
shipped state is partial:

- Horus runs as `horus supervise` with observation duties.
- Its mail routes to Ra once a build with #1194 is installed.
- Per-module versioning is not yet separate from the single `sirsi` release.
- The standalone/integrated board parity has not been audited against the Swift
  standard.
- These gaps are post-freeze work items. They are not claimed done.

## Durable contracts

1. **Authority:** all production calls resolve through `routerstore.Resolve` and
   all writes pass through `dispatch.Facade`.
2. **Identity:** a host token mints a session bound to host, agent, thread and
   runtime hash. Mutating verbs require a fresh registered thread and a live
   lease/nonce.
3. **Delivery:** a store row is durable before a wake is attempted. Wake events
   are bounded, leased, retried, and terminally escalated; `acked_at` is a
   recipient-only read acknowledgement, never a substitute for completion.
4. **Isolation:** Codex lanes use the filesystem relay; the relay is the sole
   token holder on a host and refuses token-management verbs. The relay never
   copies the router database.
5. **A2A semantics:** identity, body integrity, idempotency, correlation,
   explicit error classes, and read acknowledgement are store-backed. MCP is a
   front end, not a second protocol authority.
6. **Observation:** Horus and Stack Lab read the same service state and must
   distinguish liveness, reachability, lease state, and completion evidence.
7. **Recovery:** schema changes are transactional and forward-only; service and
   client rollback use retained, hash-checked identities and atomic replacement.

## Delivery model

The currently shipped implementation keeps the existing wake safety logic and
uses the router supervisor/relay as the host delivery boundary. A router-owned
informer is the next additive evolution: it may centralize subscription and
adapter selection, but it must preserve the existing progress gate, PID adoption,
spawn ceiling, quarantine, recipient-only acknowledgement, and no-network Codex
boundary. No informer implementation may silently reintroduce per-lane store
addresses or blind-spawn interactive sessions.

## Release boundary

This release candidate closes the one-authority regression and publishes the
complete recipe and evidence contract. It does **not** claim that every fleet
lane, a fresh third physical machine, or commercial GA has been observed in this
turn. Those are explicit evidence rows in the traceability matrix. The release
state is platform-foundation/pilot, not a claim of universal production proof.

## Ownership

- Ra owns the router implementation and this Stack Lab wing.
- Horus owns per-node observation and the node's upward routing edge (relay and
  outbox); it reports to Ra and never becomes a second ledger or authority.
- Ma'at evaluates evidence and known failures; it does not rewrite router state.
- SHA remains the hardware/control-plane reviewer for host and protected-process
  evidence.
- The owner promotes source and release artifacts. No lane may silently merge,
  overwrite, or fork the canonical ledger.
