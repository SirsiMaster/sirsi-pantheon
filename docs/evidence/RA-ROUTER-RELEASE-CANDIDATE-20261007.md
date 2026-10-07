# Ra router release candidate — 0.24.97 (deployed)

**Source branch:** `fix/router-one-authority-live-20261007`
**Base:** `origin/main` `958e7fbf`
**Deployed commit:** `a96cf51f` (published to origin)
**Clean CLI build:** `sirsi` v0.24.97, SHA-256
`323906eb0dd01d7c31a28d3ec443e01acfd09719862dfeb3c2aa0d7ff94f215c`,
34,340,482 bytes, `dirty=false` from `version --json`.
**Candidate changes:** `6f71127a`, `379d8e71`, Stack Lab canon and recipe
consolidation
**Classification:** platform-foundation/pilot

## What is delivered

The deployed candidate has one production authority. `routerstore.Resolve()` selects the
configured service; an unset service configuration is an error. `SIRSI_ROUTER_DB`
is explicit test/migration input only. Ma'at tests now pass an explicit isolated
ledger instead of inheriting the operator's service environment.

The Ra wing now contains the full architecture and release plan, including the
Cloud Run/Cloud SQL authority, authenticated RemoteStore, host relay, A2A/MCP
front end, Horus read model, wake/event/ack contracts, recovery, rollback,
threat model and traceability matrix.

## Verification contract

Run from the clean candidate checkout:

```sh
scripts/router-service/test-one-authority.sh
go test ./internal/routerstore ./internal/dispatch ./cmd/sirsi
go test -race ./internal/routerstore ./internal/dispatch
git diff --check HEAD^ HEAD
```

The first command fails closed with no file created under its temporary HOME,
then proves the authenticated `RemoteStore` round-trip against an isolated
HTTP service and passes the targeted suites. The full verification also passed
`go test ./...`, `go vet ./...`, the router-store-open audit, the changelog
self-test, and completion validation. The one-authority log SHA-256 is
`853a99af8c0837178dce7ee8b3b7b522f87c5612adf41ac9884f668061abb661`.
Verification is independent of live GCP credentials and never mutates the
canonical service.

## Honest open evidence

- The GCP deployment is now read back with the existing `claude-agent` service
  account: Cloud Run revision `sirsi-router-00024-cl2` serves 100% of traffic at
  `https://sirsi-router-6kdf4or4qq-uc.a.run.app`, image digest
  `sha256:7c465ab88ce45ee61ef4b9afec7947cb98f676161605d8172ca36c26364552fc`.
  The Cloud SQL schema job `sirsi-router-apply-schema-vj5dk` completed successfully
  at schema version 24 with the complete 17-table shape. The prior healthy revision
  `sirsi-router-00022-4d7` remains the rollback target.
- The real third-machine rehearsal, complete Codex/Claude parity across every
  lane, and commercial product/narrative closure remain open in the matrix.
- ADR-065's router-owned informer remains the next bounded implementation slice;
  the existing wake safety substrate is retained and is not replaced by an
  untested second loop.

## Rollback

The prior client binary is preserved, mode `0600`, at
`~/.sirsi/quarantine/20261007-router-auth/sirsi-router-one-authority-379d8e71`;
its old Homebrew and local symlinks were moved into the same quarantine and are
not active launch targets. All nine user LaunchAgents and Horus now invoke the
new `a96cf51f` binary. The prior healthy Cloud Run revision remains the rollback
authority in `ROUTER_STACK_LAB_RECIPE.md`.
No local archive or stranded-item store is deleted by this release candidate.
