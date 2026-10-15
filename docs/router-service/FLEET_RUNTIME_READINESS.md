# Fleet runtime readiness

Classification: platform-foundation. Operator: SSA during repair, Ra after
verified handback. Users are fleet operators deciding where work can actually
be received. The risk is mistaking configuration for a working consumer, or
marking an attended lane unreachable because it must not be auto-spawned.
This supports operational trust; pricing validation is out of scope.

`router fleet`, escalation, and the dashboard use `router.PingLane` against
the current canonical thread registry. A fresh attended session or capable
consumer can receive work even with `wake=none`. A configured launch agent
without a live consumer does not establish readiness. Stale, closed, suspended,
watch-only, unauthenticated and quarantined consumers do not become routable
because a plist exists. Human recipients are not automatic worker lanes.

The dashboard refreshes the reachability map per fleet request, atomically with
observation of that request's ledger snapshot. Activity history is preserved.
An unreadable or nil runtime result returns 502; no successful empty/healthy
board is synthesized. Registry and ledger reads are observations, not a global
transaction or a guarantee against a process exiting after the read.

No credentials, account settings, dispatch permissions, leases, worker flags or
processes are changed. This does not staff a missing lane, remove a quarantine,
prove work completion, or qualify an M5-off cloud cutover. Each real lane still
owes its own correlated queue read and response using its actual runtime.

Tests cover fresh attended and worker consumers, watcher-only, stale and closed
sessions, authentication failure, per-read readiness transitions, unavailable
evidence, and preserved activity history. Deployment requires the normal signed
whole-package rollout; source tests alone do not update an installed dashboard.

Three-home publication: repository source is authoritative; Reading Room and
Workspace mirrors are pending the integrated repair publication, not claimed
current by this source checkpoint.
