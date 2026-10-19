# Ra Router release contract

## Current successor candidate — 0.24.119

PR #1158 is merged after protected full race, PostgreSQL, build, lint, canon and
security qualification. Its loaded-image/lifecycle code is exact source
`1fa1282330234e3cc4c94deb43efed45a89cff66`; the release branch also contains the
neutral civic coordination adapter and version/changelog preparation. Bind the
final tag, source tree, package signatures/notarization and installed CLI hashes
to the actual publication, not this document's candidate label.

This is not an assertion that 0.24.119 is already published or loaded. Roll out
on M1, M5 and Studio; adopt existing controllers only at verified consumer
boundaries, then read actual loaded images and prove sequential canonical queue
responses with persisted observer heartbeats. Do not use blind loop restarts,
borrow native thread IDs or copy router/credential databases. Preserve a named
rollback image and existing healthy work.

## Historical candidate — 0.24.97

Release candidate `0.24.97` is cut from commit `ec92f527` on the exact Ra branch
after the one-authority resolver fix and the canonical Stack Lab documentation
update. The candidate
must report its stamped commit, source cleanliness, router schema ceiling, and
runtime path with `sirsi version --json`.

## Required evidence

- targeted routerstore, dispatch, CLI and race tests;
- one negative control proving missing service configuration refuses instead of
  opening a local ledger;
- one positive end-to-end service round trip using an isolated disposable store;
- recipe/static contract checks and completion proof;
- exact source commit, binary SHA-256, test log SHA-256, and rollback candidate;
- live service revision and traffic receipt when the deploy identity has the
  required read/deploy permission.

## Deployment order

1. Build from a clean tree with stamped version and commit.
2. Run the local contract suite and retain the log.
3. Apply schema only if the schema hash changed; the migration is transactional
   and re-runnable.
4. Deploy the service by immutable source identity, then read back revision,
   image digest, traffic, health and rollback target.
5. Roll clients by hash-check → stage → atomic rename; never copy over a running
   inode.
6. Verify M1/M5 relay and lane identity without copying a database or secret.
7. Publish the receipt into this wing and the canonical router; mark any missing
   host/cloud observation as open rather than inferred.

## Rollback

Service rollback returns traffic to the named retained revision. Client rollback
uses the named retained candidate and atomic rename. Schema rollback is not
destructive: forward-compatible code or the retained previous service revision
is used while a corrective migration is prepared.
