# Router Surface contract

The router now has one independent operator surface and two governed consumers:

```text
Ra / router store
        │
        ▼
Horus routerboard producer
        ├── standalone: http://127.0.0.1:8734/router
        ├── Pantheon:   Horus dashboard read-model and router link
        └── Nexus:      embedded surface or router-surface.v1 read-model
```

## Authority

Ra remains the authority for work items, leases, thread state, lane verdicts and
queue counts. The frontend never opens SQLite, reads `agents.json` directly, or
recomputes a lane verdict. `routerboard.Board` is the one producer for the
standalone surface. Horus and Nexus consume projections of that producer.

## Independent surface

`sirsi board-serve` serves the existing owner-facing board at `/router`. The
page is cache-disabled and emits `sirsi.router.ready` when embedded. The parent
may use that event for layout only; it cannot issue a router mutation through
the embedded page.

## Versioned read contract

Consumers discover the surface at:

```text
GET /api/router/v1/manifest
```

The manifest identifies `router-surface.v1`, authority `ra`, the producer build,
and the canonical endpoints:

| Endpoint | Purpose |
|---|---|
| `/api/router/v1/snapshot` | Complete last-good board payload |
| `/api/router/v1/stream` | SSE updates when the payload changes |
| `/api/router/v1/ledger` | Ledger projection used by existing board consumers |
| `/api/router/v1/tasks` | Task projection used by existing board consumers |

Before the first successful poll, snapshot and projection endpoints return 503;
they never manufacture an empty board. Clients retain the last good payload and
show its generation time when the producer is unavailable.

## Surface bindings

- **Pantheon/Horus** continues to own local node operation and renders the
  router read model from its existing `/api/router` and `/api/fleet` producers.
  The standalone board is an additional door onto the same producer, not a
  second ledger.
- **Nexus** may embed `/router?surface=nexus` when the local Pantheon endpoint is
  available, or consume the versioned read endpoints directly. If the endpoint
  is absent, Nexus shows `Router surface unavailable` rather than falling back
  to a local router database or a stale snapshot.

Browser access is allowlisted for the local Pantheon/Horus ports, the Nexus
development port, and `https://sirsi.ai`. There is no wildcard CORS grant and
the contract is read-only; mutations remain authenticated router verbs.

## Non-goals

This contract does not move router authority into Nexus, Pantheon, or the page;
does not expose bearer tokens to JavaScript; and does not turn an iframe into a
second agent store. A future authenticated action surface must be added as a
separate versioned contract with the existing session, nonce, runtime and Rule
of Ra fences intact.
