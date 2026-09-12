<!-- agent: ra | workstream: router-service (ADR-062) | Stack Lab wing: stacklab.wing.m1-ra -->

# Router Stack Lab Wing — `stacklab.wing.m1-ra`

The router's entry in the Sirsi Stack Lab catalogue, in SNE's authoritative
wing format. This is a **hash-pinned reference**, not a second authority: the
schema and fixtures are owned by the SNE lane (`sirsi-inference`); this document
pins them by SHA256 and records the router wing that validates against them.

## Authority & provenance (do not fork the schema here)

The wing **schema** authority lives in the SNE repo and is consumed by hash, not
copied into Pantheon as an editable second authority (per codex-inference,
2026-09-11):

| Artifact | Location (authority: `sirsi-inference`) | SHA256 |
|---|---|---|
| `wing.schema.json` | `contracts/stacklab/v2/wing.schema.json` | `a69e0094b8ec4596c30b316fc8bd6ce5f806f5a801b5c80c10a972e3d86338ad` |
| `router-wing-ra-v1.json` (this wing's record) | `contracts/stacklab/v2/fixtures/router-wing-ra-v1.json` | `f03456822b36c0649f34e474c23e74bc4a1020d6484195458ae781df99095ce7` |
| `reject-router-wing-cross-project-write.json` (negative control) | `contracts/stacklab/v2/fixtures/reject-router-wing-cross-project-write.json` | `cf69bca52c39b4598759fde1ac2926a478ada1a3f01c8ccb1ae9b046cef089ad` |

The wing record `router-wing-ra-v1.json` is vendored **beside this file** as
provenance (byte-identical to the SNE fixture, SHA256 `f0345682…`). To refresh,
re-fetch from the SNE authority and re-verify the hash — never hand-edit the
vendored copy into drift.

## The wing

- **id**: `stacklab.wing.m1-ra` · **class**: `control-plane` · **owner**: `ra`
- **project**: `sirsi-pantheon` · **namespace**: `router`
- **first gate**: `RA-WING-001.G1` — host-neutral router-authority receipt and no competing replica proof
- **component catalog**: `docs/router-service/ROUTER_STACK_LAB_RECIPE.md@6eaa89b4b8cbaa659cdef3d1b85e94ec2b59f851` (the SSA-accepted operating inventory, PR #736)
- **scope**: router replication, constrained-client parity, worker-plane visibility, recovery receipts. **Does NOT own** SNE source, models, model stores, or engine promotion.

### Lifecycle (per SNE convention)

`recipe-declared` → `built` → `native-exec` → `mlx-parity` → `perf-matched` → `promoted` → `retired`

Router items are immutable material-state receipts; the phase is independent of
the Stack Lab evidence state (a rejected result keeps its lifecycle + evidence
rather than disappearing). Tracked as router task `ra-wing-router-v1`.

## Enforcement invariant — schema shape is NOT an authority grant

The schema's own `$comment` is load-bearing:

> Router enforcement must additionally prove every writable root is inside the
> declared repository or the declared project/namespace evidence root, and must
> reject all other paths.

A record can be schema-valid and still request authority it must not receive.
In this very record, `workspace.writable_roots` includes

```
/Users/thekryptodragon/Library/Application Support/Sirsi/RouterBackups/sirsi-pantheon/router
```

which is **outside** the declared `repository_root` and `evidence_root`. Under
this schema version a strict runtime MUST reject that root **unconditionally** —
schema validation alone does not authorize it, and this reference grants no
exception. Any future authority for an extra writable root would require its own
explicit, versioned contract, never an implied exception here.

Two distinct adversaries, do not conflate them:

- **Shape-invalid (the shipped `reject-router-wing-cross-project-write.json`,
  `cf69bca5…`)**: it adds an unknown `workspace.cross_project_write_root`, so the
  schema's `additionalProperties:false` rejects it at **validation**, before any
  runtime ownership check. Its `writable_roots` holds only the Pantheon repo.
- **Schema-valid adversary (not shipped here)**: a foreign path placed directly
  in `writable_roots` with no unknown field — it passes schema shape and MUST be
  rejected at **runtime** on ownership. Pinning this derived fixture and the
  runtime rejection is **rs-31 future work**; the authored `cf69bca5…` bytes are
  preserved unchanged as provenance.

## Required Router behaviors (SNE) and status

1. Validate every wing record against the schema — **tracked: rs-31**.
2. Enforce project/namespace ownership of writable roots — **tracked: rs-31**
   (unconditional default-deny for any root outside repo/evidence root). Note the
   shipped `cf69bca5…` fixture is denied by the **schema** (unknown property); the
   runtime-ownership test needs a separate schema-valid adversary fixture (a
   foreign path in `writable_roots`, no unknown field) — that derived fixture is
   rs-31 work. The authored SNE bytes are preserved unchanged as provenance.
3. Show task phase, owner, blocked-by links, and receipt links in the worker
   plane without duplicating recipe or payload bytes — **tracked: rs-31**.
4. Treat quota backpressure as a throttle/counter, not a delivery-breaker
   failure; every breaker trip carries a retrievable cause receipt —
   **DONE: PR #737 MERGED to main (squash `c9d6c8e4`)** (quota decoupled from the
   breaker; `escalateTx` returns a retrievable cause id stored on the breaker row).

## Validation receipt

`router-wing-ra-v1.json` (SHA256 `f0345682…`) structurally validates against
`wing.schema.json` (SHA256 `a69e0094…`): all 14 required fields present;
`schema`=`sirsi.stacklab.wing.v1`; `class`/`status` in enum; `boundary_policy`
=`default-deny`; `handoffs` `router-receipt-only`. The independent enforcement
check flags the one foreign writable root named above (as required).
