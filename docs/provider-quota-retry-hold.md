# Provider quota retry hold

Refs: ADR-061 durable delivery; AGENTS.md Lean11.

An explicit monthly-spend-limit diagnostic from a failed consumer is a provider availability failure, not completed work or authentication failure. Generic429s, quoted messages, item prose and ambiguous reset text are not sufficient. A successful consumer retains existing progress ordering.

The implemented local per-lane record uses the existing dispatch directory, survives controller restart and has no guessed reset timer. While held, the worker publishes PROVIDER_QUOTA with workable=false and does not dispatch. Existing attended sessions retain custody; no provider substitution, budget/auth change, lead rename or service lifecycle action is introduced. Existing spawn ceilings, admission, circuit breakers and fruitless quarantine remain.

After independently verifying legitimate provider availability, the local lifecycle custodian may use `sirsi router provider-available <agent> --generation <observed-failure-generation> --evidence <nonsecret-reference>`. This records availability rather than changing account limits. A newer quota failure blocks again; prior failure timestamps and last evidence remain. Do not run this merely because a controller restarted or a guessed billing date arrived.

## Source integration and qualification history

Cloud Ra produced the source on the then-current main fcb4dbad but its retained Go1.25
could not compile the Go>=1.26.2 project. That failure is preserved, not treated
as a product failure or bypassed by downgrading the module. SSA reproduced all
v2 file hashes in an isolated native checkout with Go1.27.1 and obtained focused
test PASS188dee98. Two additional source cures preserve real progress while
independently arming a failed provider hold, and bound lock acquisition to200ms
rather than allowing a stuck lock to stall controller heartbeats indefinitely.
Stable focused race controls passed32e09ab6, including concurrent generation
recovery, the real Unicode diagnostic, corrupt/unwritable storage and bounded
lock contention. At that October20 checkpoint, full affected-package race
qualification was still running; the checkpoint is historical, not a current
job-status signal. This document does not establish that run's terminal result.

Later protected CI supplies a distinct, completed qualification receipt:
[PR1257 Test job123476885508](https://github.com/SirsiMaster/sirsi-pantheon/actions/runs/40882340139/job/123476885508)
on exact source `60644218ce1de5c72cf23e19f35a742464832c15` passed all seven
`TestProviderQuota*` functions with the repository command
`go test -v -race -short -coverprofile=coverage.out ./...`; the router package
completed successfully. Both `providerquota.go` and `providerquota_test.go`
have the same immutable Git blob IDs there as at the integrated unlock-error
correction `d0e04220c342ac0fe7924afeb9c889a92c37a971`. This is actual existing
CI evidence for that corrected quota source under race/short mode, not a new
helper test run or a claim that every non-short/native-provider scenario passed.

The implementation landed in commit
`1d324cf859919e64333578cffe395e2753349085`, followed by unlock-error handling
and import corrections in `d0e04220c342ac0fe7924afeb9c889a92c37a971`.
Both are ancestors of the v0.24.143 release source
`e34f1fb2a5c064c4a20c23ede3d79930ba94ad75`. Source integration is therefore
not an outstanding blocker for this slice. A published package does not prove
that every controller has adopted it, that a particular lane's hold was
exercised after restart, or that its provider became available. Those runtime
claims still require their own exact receipts; do not clear a hold from release
or installation evidence alone.

The record is host-local dispatch metadata, not another router database or
global account authority. A failed durable write is reported as a blocked
observer and an error, not a claim that a nonexistent durable hold survived a
crash. No timer, weekly-reset prose, controller restart or source test establishes
that provider capacity recovered. No Mac/provider call is used by these fixtures.

## Recipe and operations

- Component: `ra.provider-quota-retry-hold` in the Ra Stack Lab wing.
- Path: consumer exit facts → anchored diagnostic → persisted failure generation
  → non-workable published lane → no new dispatch; actual successful earlier
  queue actions remain recorded independently.
- Recovery: inspect the current failure generation and independently obtained
  provider availability evidence, then use the typed `provider-available` command.
  A stale generation cannot clear a newer failure. This command does not enroll
  credentials or alter provider spending limits.
- Release: normal source review, protected CI, signed packaging and safe idle
  controller adoption. Preserve currently executing consumers, old failure logs
  and registry/role ownership. Runtime suppression proof remains separate.
- Rollback: retain the prior signed artifact and durable quota history; reverting
  code must never be described as fixing the upstream provider spending cap.

The repository is the canonical source. Owner Reading Room and native Workspace
mirror synchronization is not established by the source/release evidence above.
Three-home publication remains unproven until their existing linked copies and
this canonical revision are reconciled; do not infer synchronization from a
package release.
