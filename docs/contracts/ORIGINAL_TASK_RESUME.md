# Original-row attempt-preserving resume

Implementation contract for Main/Ra review; not a deployed capability or a new
permission grant. Refs ADR-061/062; Ra canonical193750 (2026-10-24) identifies
the missing supported operation, not authority to act on another lane.

`ResumeTaskClaim(ctx, request)` has two explicit v1 actions: `inspect` and
`claim`. Inspection returns an opaque SHA256 version of every persisted task
column, including custody and scope, without exposing old lease tokens.
This is an exact persisted-snapshot digest, not a new monotonic schema column
or a claim to detect a historical ABA that restores every persisted byte.
Claim requires that exact version, original agent/task identity, a cause-fixed
reason and evidence reference/SHA256. Evidence is authenticated actor
attestation, not server filesystem verification or independent approval.

The service derives actor/host/thread/runtime from its authenticated session,
requires a fresh own-lane registered thread even if the general gate is off,
and rechecks it under transaction locks. No operator-token foreign-lane bypass.
Only exhausted original rows in pending/in-progress/blocked, with empty
blocked_by, qualify. Like explicit normal ClaimTask, responsible_party is
preserved; the automatic wake's dispatchability filter is not a new manual
permission gate. Task agent must still equal the authenticated actor.
Live, partial or malformed
custody refuses. Complete valid expired custody may be replaced, never reused.

The operation atomically issues ONE ordinary bounded task lease, increments
attempts (3 becomes 4; never zero), binds its session and acknowledges only its
exact wake source. It preserves failure_reason, charter, timestamps/history,
links, counters, scope and result_ref except the normal lease/status/updated
fields. Append-only reserved provenance records before/after versions,
attempts, old holder/expiry/failure, evidence and authenticated actor; never a
lease secret. Failure of audit/binding/wake acknowledgement rolls everything back.
Exact retry may return the same still-live lease only for the same request and
same authenticated worker, while the exact recorded after-version remains;
it never starts another attempt. Changed or completed after-state refuses.

Completion/renew/release remain the existing authenticated token-fenced APIs.
There is no settlement-to-DONE, generic update widening, attempt refund, new
table/schema, role/key, copied task, implicit local authority or automatic
retry. Ordinary claim ceiling remains unchanged. A new explicit cause-fixed
request is needed after any further failure, preserving that failure too.

CLI: `sirsi router task resume-claim AGENT TASK` inspects by default; `--apply`
requires `--expected-version`, `--evidence-ref`, `--evidence-sha256`, `--reason`
and normal bounded `--ttl`. CLI uses configured canonical RemoteStore only.
The normal 30-minute ceiling is retained for recovery claims and renewal;
45 minutes refuses before mutation. New-operation validation has the typed
`ErrTaskResumeInput` wire error and HTTP400; old-method error mapping is not
changed by this correction.
No runtime application is authorized by source preparation or qualification.

Qualification must cover SQLite and the existing isolated-PG harness, real
authenticated RPC and CLI, unchanged schemas, CAS/concurrency, exact replay,
stale tokens, live/malformed custody, foreign/revoked/stale actors, audit
failure rollback and unchanged old-client claim/update/complete behavior.

Deploy the compatible canonical server before using the new client operation;
an old server's missing method refuses, with no reset, generic update or local
DB fallback. Existing old-client methods and migrations are unchanged. Source
and fixtures are authored only: no live original has been resumed or settled.

## Authored qualification matrix (not executed)

`./internal/routerstore -run '^TestTaskResume'` selects history/normal completion,
eligible-state and principal refusals, audit rollback/contention, stale snapshot
and changed replay, unchanged legacy metadata/renew/failure path, real scoped
HTTP caller/reminted completion, strict request schema/raw wire bound and
cancellation. `TestTaskResumePostgresHeldRowDeadlineBeforeUnlock` must run on
the existing **fresh owned** PG harness, never a shared DSN (that harness drops
its schema); SQLite explicitly skips only that PG-specific lock fixture.
Run the same package cases normal/race on both backends. Existing task claim,
update, reset and lease-fencing baselines remain required compatibility controls.

`./cmd/sirsi -run '^TestTaskResumeCLI'` invokes actual Cobra parsing/RunE with a
real private authenticated loopback Handler: default inspect, explicit fresh
claim and separate normal completion, malformed input before any authority,
and refusal of a local-store fallback. Fixtures create only disposable owned
stores; neither CLI nor server contacts the live ledger.

Causal production-only controls, unchanged fixtures (compile required):

1. Remove only the stale row-version comparison; the scope-change assertion in
   `TestTaskResumeStaleVersionAndChangedReplayRefuse` must fail.
2. Change only `expiry.After(now)` to `expiry.After(now) && false`; the
   `live-holder` case in `TestTaskResumeRefusesIneligibleAndForeignPrincipals`
   must fail (valid expiry parse remains, so this is not an unused-variable RED).
3. Change only `attempts=attempts+1` to `attempts=0` in the recovery UPDATE;
   `TestTaskResumePreservesOriginalHistoryAndNormalCompletion` must fail its
   actual persisted attempt/history assertion. No fixture or normal API changes.

Static gofmt/diff checks do not constitute any of these executions. Source
review and Main's normal admission precede qualification or canonical use.
