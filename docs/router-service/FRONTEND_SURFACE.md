# Router Surface contract

The router now has one independent operator surface and two governed consumers:

```text
Ra / router service
        │
        ▼
Pantheon/Horus supervised server :9119
        ├── standalone: http://127.0.0.1:9119/router
        ├── Pantheon:   same RouterFn read model
        └── Nexus:      allowlisted read-only router-surface.v1 consumer
```

## Authority

Ra remains the authority for work items, leases, thread state, lane verdicts and
queue counts. The frontend never opens SQLite, reads `agents.json` directly, or
recomputes a lane verdict. `dashboard.Config.RouterFn` is the one producer for
the standalone surface, Pantheon, and Nexus.

## Independent surface

The supervised `sirsi dashboard` process serves the owner-facing surface at
`/router`. It reads the same typed `RouterFn` used by Horus; there is no second
shell-polling board process to start, drift, or silently disappear. The page is
cache-disabled and shows an explicit unavailable state when the producer fails.

## Versioned read contract

Consumers discover the surface at:

```text
GET /api/router/v1/manifest
```

The manifest identifies `router-surface.v1`, authority `ra`, the producer build,
and the canonical endpoints:

| Endpoint | Purpose |
|---|---|
| `/api/router/v1/snapshot` | Typed last-good `RouterSnapshot` |

An unconfigured producer returns 503; a collection failure returns 502. It never
manufactures an empty board. The console labels a retained last-good snapshot
with its original generation time and a stale/unavailable banner.

The `/router` and `/` doors now serve the same embedded operator workspace,
including searchable lanes, inspection, queue, fleet, releases and host views.
The manifest includes the running executable's build identity. The actual local
deployment and rollback contract is in `FRONTEND_DELIVERY_20261008.md`.

## Surface bindings

- **Pantheon/Horus** owns local node operation and exposes the same `RouterFn`
  through `/api/router` and the versioned surface endpoint.
- **Nexus** consumes `/api/router/v1/manifest` and `/api/router/v1/snapshot`
  directly. If Horus is absent, Nexus shows `Router surface unavailable`
  rather than falling back to a local router database or a stale snapshot.

Browser access is allowlisted for the local Pantheon/Horus ports, the Nexus
development port, and `https://sirsi.ai`. There is no wildcard CORS grant and
the contract is read-only; mutations remain authenticated router verbs.

## Non-goals

This contract does not move router authority into Nexus, Pantheon, or the page;
does not expose bearer tokens to JavaScript; and does not turn an iframe into a
second agent store. A future authenticated action surface must be added as a
separate versioned contract with the existing session, nonce, runtime and Rule
of Ra fences intact.
