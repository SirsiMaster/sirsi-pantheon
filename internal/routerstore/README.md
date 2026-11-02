# internal/routerstore — the router ledger (developer README)

`routerstore` is the durable store behind every `sirsi router` verb: items
(inbox messages), the task ledger, leases, wake events, threads, breakers,
quotas, identifiers, requirements — and since ADR-062, sessions and host
tokens. One package, one `Store` interface, three ways to reach it.

## Shape (ADR-062)

```
cmd/sirsi verbs ──► dispatch.Facade ──► routerstore.Store (interface)
                                          ├── *SQLiteStore + sqlite dialect   (Anubis: ~/.sirsi/router.db)
                                          ├── *SQLiteStore + postgres dialect (Ra: OpenPostgres, pg/schema.sql)
                                          └── *RemoteStore                    (a node on the service, SIRSI_ROUTER_URL)
                                                    │ HTTPS, signed
                                                    ▼
                                          sirsi router serve ──► Handler(Store) ──► one of the two above
```

- **`Store` interface** (`store_iface.go`): the contract. Generated from the
  exported methods of `SQLiteStore`; `var _ Store = (*SQLiteStore)(nil)` and
  `var _ Store = (*RemoteStore)(nil)` fail the build on drift.
- **`Resolve()`** (`resolve.go`): the ONLY production constructor.
  `SIRSI_ROUTER_URL` (+ `SIRSI_ROUTER_TOKEN`) → `RemoteStore`; `SIRSI_ROUTER_DB`
  → that SQLite path; else `~/.sirsi/router.db`. `OpenPath`, `OpenPostgres`,
  `OpenReadOnly` exist for tests and the service entry point; the direct-open
  gate (`scripts/check-router-store-open.sh`, CI + pre-push) fails the build on
  any other caller. `LocalPath()` is read-only diagnostics and refuses under a
  service URL.
- **Dialect seam** (`dialect.go`): `dbHandle`/`txHandle` wrap `*sql.DB`/
  `*sql.Tx` so every statement passes through one `rewrite`. SQLite = identity.
  Postgres = `?`→`$n` (string literals skipped), `INSERT OR IGNORE`→
  `ON CONFLICT DO NOTHING`, `strftime(...)`→`router.now_rfc3339()`,
  `lower(hex(randomblob(16)))`→`router.rand_hex32()`; engine-classified retry;
  `FOR UPDATE SKIP LOCKED` appended to the claim SELECT. Keep it in lockstep
  with `pg/schema.sql` (table in `pg/README.md`).
- **Service** (`serve.go`): reflective handler at `POST /v1/call/{Method}`.
  Auth chain, in order: host bearer (bootstrap or per-host token) → session →
  nonce (±60 s, single use) → HMAC signature → runtime hash bound at mint
  (mismatch revokes) → ownership on lease-bearing mutations. Store failures in
  the chain are 503, never 401. `notServed` lists server-only methods.
- **Client** (`remote.go`, generated `remote_gen.go`): `RemoteStore`. Mints and
  caches a session (`~/.sirsi/sessions/<agent>.json`, 0600), signs every
  request, long-polls `Wait`, treats 503 as `ErrServiceUnavailable`.
- **Identity** (`sessions.go`, `hosttokens.go`): `sessions`, `lease_sessions`
  (side table — items stay field-for-field `work.Item`), `host_tokens`
  (SHA-256 only).
- **Migration** (`migratestore.go`): `CanonicalDump` (all 14 tables, PK order,
  hashed) and `MigrateStore` (DO NOTHING copy, source re-dump guard,
  destination hash must equal source, trigger-minted extras removed, NUL
  scrub on request). CLI: `sirsi router migrate-store`.

## Adding or changing a `Store` method

1. Implement it on `*SQLiteStore` (one file per concern).
2. Regenerate `store_iface.go` and `remote_gen.go` — the generator is the
   small script in the rs-09/rs-10 commit messages' lineage; until it is
   checked in as `go generate`, run the same transformation: every exported
   `func (s *SQLiteStore) X(...)` becomes an interface line and a
   `RemoteStore` stub calling `rs.call("X", []any{args...}, &out...)`.
   `Wait`, `ListenNotify`, `NotifyAgent`, `NotifyPath`, `Close` are
   hand-written.
3. If the method must never be callable from a node, add it to `notServed`.
4. If it takes a lease and mutates, add it to `itemOwnership`/`taskOwnership`.
5. If it returns a new sentinel error, add it to `sentinelErrors` and the
   pinned list in `TestSentinelsRoundTrip`.

## Tests

### Authenticated task claims

Authenticated ClaimTask/ClaimNextTask require the requested agent/thread to
match this caller's nonempty authenticated session thread, including when the
general audience policy is in log/off mode. A body `--thread` is not a caller
binding. The internal `claimTaskSession` transaction commits the exact session
owner together with task lease, attempt and wake acknowledgment; a binding
failure rolls all of them back. No new RPC, schema or credentials are added.
Existing release/renew/complete checks and same-host/agent/thread remint rules
are unchanged. Pre-existing threadless leases are not transferred or repaired
by registering a different session. The negative regressions are in
`taskclaim_session_binding_test.go`; `remote_test.go` preserves legacy cache
and existing-lease ownership behavior without authorizing new threadless claims.

### Authenticated item claims (DC01)

`serve.go` routes authenticated ClaimNext/ClaimItem to the unexported backend
`claimItemSession` capability. `lease.go` shares existing selection/budget/fence
transactions with local callers, inserting durable owner binding before commit.
No new Store/RPC method or schema is added. Wrappers used as server backends must
explicitly retain this internal capability or authenticated claims fail closed
before mutation; they must not emulate it with ClaimItem plus BindItemSession.
Tests in `claim_session_atomic_test.go` use the real authenticated handler and
registered threads with SQL-fault injection, not a mocked successful bind.


- `go test ./internal/routerstore/` — SQLite (161 tests).
- `SIRSI_TEST_PG_DSN=postgres://sirsi@127.0.0.1:54329/routerstore_test go test ./internal/routerstore/`
  — the same suite on Postgres with a per-test schema reset from the
  embedded `pg/schema.sql` (two tests skip: they introspect the SQLite
  catalog; `scripts/check-pg-schema.sh` covers the Postgres side).
- `bash scripts/ci-postgres.sh` — what CI runs: scratch cluster, schema
  script (with negative controls), full suite.
- `sirsi router bench` — the two-host evidence run; see
  `docs/evidence/ADR-062-RS13-TWO-MAC-EVIDENCE-20260902.md`.

## Known limitations

- `SQLiteStore` is a stale name for a dialect-carrying store; renaming is
  cosmetic and deferred.
- Runtime identity is the executable's self-reported SHA-256, bound at mint;
  release-manifest attestation is Phase D (rs-16).
- `ListenNotify` over HTTP is a polling goroutine, not a FIFO.
- The migration tool is designed for SQLite → Postgres; other directions use
  the same code but are untested at scale.

## Exact thread retirement (ADR-057 N2)

`DeleteThreadCAS` and its scoped variant keep their API names and boolean CAS
result, but retire the exact observed row to reserved `pruned` rather than
physically deleting its primary key. Agent, host and observed timestamp remain;
the user payload becomes `{}`. Ordinary list/get hide the fence, while the
authority binding retains its non-live identity. New unrelated IDs still work;
normal registration, resume and imports cannot create or revive a pruned ID.
There is no fence garbage collection or new store/schema/RPC.

Both backends use the same production retirement statement. The two-handle
fixtures call the real scoped/unscoped methods; PostgreSQL contention holds that
statement uncommitted while actual peer mutations wait. A separate raw physical
delete demonstrates the legacy resurrection defect, not a production path.

Before deploying a G2 writer to canonical storage, Ra/SSA must establish the
compatible G1 server/direct-writer and rollback floor, including in-flight and
rejoining writers. A new app or a successful disposable-store test does not
prove that deployment condition. Do not prune live sessions to qualify this fix.
