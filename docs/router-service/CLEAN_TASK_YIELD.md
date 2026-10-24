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
repeated clean yields. Unknown RPC on older servers is refusal: no fallback to
failure release or automatic reset, and no rewrite of historical failures.

Tests reproduce fourth-phase exhaustion with legacy release, then verify
repeated typed yields resume; prior/genuine failures still exhaust; stale and
concurrent refunds are fenced; foreign sessions are denied; audit failure is
atomic. Shared backend helpers allow isolated PostgreSQL qualification, never
a production/shared DSN. This cloud workspace has no PG fixture or private SDK
refresh authorization. Native/hosted validation, normal integration/release
packaging and StackLab record remain required at SSA's sole publisher boundary.
Parent clean-yield-retry-accounting-20261024 remains open until those outcomes.
