# Gemma worker replacement: canonical claim barrier

Source correction on released 0.24.128; not activated by installation.

`Store.LaneClaimBarrier(ctx, request)` and its matching RemoteStore RPC accept
only registered, fresh `sirsi-software-admin` sessions for `gemma-pantheon`.
No caller-supplied actor, new role or credential is involved. Ra cannot activate
this foreign-lane operation. Both item claims and both task claims serialize
with acquisition on one durable state row. Other lanes bypass that row.
Generic state writes/CAS cannot overwrite it.

The controller generates a cryptographically random 128-bit hex token and
retains it in normal process memory BEFORE acquire. Same-owner/same-token
acquire is idempotent after an uncertain response. Never print/cache the token.
Owner binds agent, authenticated host and registered thread; another thread
cannot release it. Status/release require the capability. No automatic expiry
can resume the old worker midway through replacement. Ordinary operator exits
must release in a finally block after reconciling their exact outcome; this is
not crash recovery. No token is checkpointed to disk.

The operator must bind the barrier to its actual dedicated controller process
registration, not borrow a still-working parent app thread. If the barrier is
bound to a live parent, recovery cannot falsely close that parent to proceed.
For controller loss, the same authenticated SSA host/agent first verifies the
old controller process has terminated and closes ONLY its exact owned
registration through the existing normal lifecycle. A fresh registered SSA
thread on that same host sends `recover`, naming `previous_thread` and a new
random capability retained in memory before the request. The store requires
that exact previous registration to be CLOSED with matching agent and host,
and atomically transfers ownership/rotates capability while keeping the lane
paused. Active, idle, blocked, suspended, unknown or merely stale controllers
cannot be recovered. The old capability loses authority. Same recovery request
is idempotent after a lost response. Another host, agent, prior thread or token
cannot take over. Recovery does not reclaim or settle work. Native process
termination evidence remains operator custody: a CLOSED row alone is not OS
proof. Missing evidence means defer while preserving the barrier, never fake
closure, generic force release or automatic timeout.

After acquire, status returns active item and task lease counts in the locked
transaction. Existing renewal/completion/failure paths remain unchanged. Counts
include expired/unsettled rows; no lease reclamation is performed by the barrier.
Zero counts prove canonical executable leases absent at that read, NOT physical
worker idle or subprocess termination. SSA must still reconcile its exact owned
worker and use the separately qualified lifecycle controller while holding the
barrier. Retain the same controller/thread, verify replacement and release
explicitly on every normal outcome. Never stop foreign payloads or spoof failure.

Rollout order: qualify this server plus matching typed client first; require
ALL authoritative canonical writers/routes at this correction or newer before
activation. Old ClaimNext/ClaimItem clients work against the corrected server
without modification; an old server is NOT barrier-safe. Once activated, rollback
to an older claim server is unsafe until the controller releases the barrier and
settles the maintenance operation. Unknown RPC on old servers is a refusal,
never permission to fall back to local state or circuit breakers. This adds no
SQL schema migration. PostgreSQL qualification must execute the same adversarial
suite with its isolated test DSN before deployment; SQLite PASS is not PG PASS.

Operator request actions are acquire, status, recover, release. Lane is always exactly
`gemma-pantheon`. Controller operations have a five-second transaction context;
external commands remain bounded. This source packet does not implement or
claim SSA's selected launchd worker replacement, M5 root refresh, automatic wake,
M5-off continuity, or completion of original BI72.

## Worker compatibility and qualification

Both server and new client include ErrLanePaused in the closed wire sentinel
map. The ordinary new Gemma worker handles it as idle on BOTH exact/next claims:
no execution, failure, retry-attempt consumption or reply; the existing loop
retains heartbeat and its context-cancelable 30-second idle wait. Once mode
returns normal no-work. Real broker/transport errors retain existing behavior.
Old worker errors return nonzero after current work settles and it reaches a
claim. SSA reports actual managed KeepAlive SuccessfulExit=false. That permits
an error-exit restart; actual selected ProgramArguments, installed image and new
kernel PID/start/inode must still be joined before claiming natural adoption.
No stop/model-broker action or raw signal is part of this protocol.

Source tests cover four independent-connection claim races with per-race clean
databases, HTTP typed refusal for both item methods, wrong actor/lane/capability,
current-lease settlement, exact-owner recovery/refusal, idempotent recovery,
old-capability revocation, and worker no-side-effects when paused.
Run shared routerstore tests with an explicitly provisioned isolated
SIRSI_TEST_PG_DSN to exercise PostgreSQL. No PG server/DSN was available in the
cloud source workspace; PG PASS remains required before activation. cmd/sirsi
worker test is authored but local compilation is blocked by absent private
sirsi-inference module with GOPROXY=off; no dependency refresh or stub allowed.
The native custodian must execute that test against its normal exact SDK.
Release129 proceeds independently; this packet is a later corrective, not129
qualification or permission to deploy the barrier alone.
