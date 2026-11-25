# Clean task yields and retry accounting

Source correction from released130 60b9963d, not a deployed change.

Claims charge the retry budget up front. The legacy `ReleaseTaskLease` remains
a failure release: its reason is never parsed, even if it says “clean wait”.
Failures, expiry and dead-letter ceilings retain their existing behavior.

New `YieldTaskLease(agent, taskID, token, disposition, evidenceRef, dependency)`
requires the exact live fenced task lease and authenticated claiming session.
Disposition is explicitly `clean` or `external-wait`. Clean has no new blocker;
external-wait requires one. An existing different dependency is refused, never
cleared. A dependency changed concurrently loses the CAS rather than being
replaced. All preexisting failure history remains intact.

A successful yield refunds exactly the current claim (attempts minus one),
clears only its lease ownership, returns to pending, and appends a typed yield
receipt to the existing task_lease_log in the SAME transaction. Prior failures
remain charged. Expired, foreign and replayed capabilities cannot refund;
concurrent callers cannot double-refund. Audit failure rolls the whole yield
back. No schema migration, automatic reset, reason regex or role grant.

Compatible server/client rollout must precede use. Exact new command:

    sirsi router task yield ra TASK --lease CAPABILITY --disposition clean --evidence REPLY_ID
    sirsi router task yield ra TASK --lease CAPABILITY --disposition external-wait --evidence REPLY_ID --dependency DEPENDENCY

Keep the capability only in normal runtime memory, not evidence or shell logs.
The existing CLI lease argument convention is retained; a future capability
transport improvement is not implemented here. Result evidence describes the
partial outcome; yield NEVER completes the original. On return, resolve only
that owner's genuinely satisfied dependency through the normal metadata API,
then claim the SAME original task normally. No reset-attempts is needed for
a few clean yields. Bound (Ra, 2026-11-25; not part of the SSA-accepted source):
at most `MaxRefundedYields` (3) refunded yields per task since the last
unrefunded one, counted from the `verdict='yielded'` rows of `task_lease_log`
(no schema change). The next yield keeps its attempt charged, is logged as
`yield-bounded`, opens a new window, and blocks the task at the retry ceiling.
Failure releases and lease expiries do not reset the window (conservative). Unknown RPC on older servers is refusal: no fallback to
failure release or automatic reset, and no rewrite of historical failures.

Tests reproduce fourth-phase exhaustion with legacy release, then verify
repeated typed yields resume; prior/genuine failures still exhaust; stale and
concurrent refunds are fenced; foreign sessions are denied; audit failure is
atomic. Shared backend helpers allow isolated PostgreSQL qualification, never
a production/shared DSN. This cloud workspace has no PG fixture or private SDK
refresh authorization. Native/hosted validation, normal integration/release
packaging and StackLab record remain required at SSA's sole publisher boundary.
Parent clean-yield-retry-accounting-20261024 remains open until those outcomes.

## Actual caller adoption and compatibility boundary

`router task release` remains failure by default. Explicit `--disposition clean`
or `--disposition external-wait`, together with `--evidence` (and a wait's
`--dependency`), now dispatch through `SettleTaskAttempt`. `router task yield`
uses the same dispatch. Failure reasons are never classified. Unknown intent,
expiry, or mixed failure/clean fields refuse rather than refund.

The real Codex Apollo consumer backstop prompt now uses that explicit clean
handoff only for its own already-held live lease when the attended owner takes
over. It never claims a foreign or forbidden task to yield it. Other consumer
exit codes and unstructured model output are NOT interpreted as clean waits;
no daemon with a typed task-outcome channel was found to wire automatically.
External operator helpers must emit the explicit outcome flags after qualified
rollout. Existing unknown external helpers are not silently adopted.

The additive own-scope `TaskYieldCapabilities(agent)` RPC returns protocol1,
clean/external-wait support, current-attempt-only and atomic-audit properties.
Clean/wait callers require this exact compatible response BEFORE mutation.
Old-server404, a malformed/incompatible capability, or an old mutation route
fails closed; there is no failure-release/reset fallback. A capability response
is NOT proof that all weighted/alias routes are upgraded. SSA must qualify
server and caller binaries/registry together and ensure all writable routes
support this protocol before activating caller adoption. Legacy failure callers
remain compatible. A transport-uncertain yield still needs exact own-task/audit
readback, not an assumption that the lease remains held or a blind retry.

No live registry refresh, service rollout, server deployment, failure refund,
expired-lease refund or historical row reset is performed by this source delta.
This is not autonomous queue recovery or whole-original completion.
