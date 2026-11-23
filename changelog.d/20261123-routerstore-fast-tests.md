### Changed
- `internal/routerstore` test suite: fresh SQLite stores are seeded from one already-migrated template (built once per test process by the real `OpenPath`) instead of replaying every schema migration per test, large fixtures insert in one transaction, and the 1,000-round claim contention test runs as four parallel shards of 250 rounds each. A new test compares the template with a freshly migrated store (schema, `user_version`, seed rows) so it cannot hide a migration bug; migration and open-from-empty tests still run the real path. `go test -race -short ./internal/routerstore` drops from about 510 s to under 150 s. Test-only; production behavior is unchanged.

Refs: `internal/routerstore/fasttemplate_test.go`
