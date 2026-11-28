# Router Fabric read

`sirsi router fabric` prints one JSON `FabricBoard` read shared with the
dashboard surface. It is read-only and returns an error without printing a
partial board when any required read fails.

The command passes its cancellation and deadline context through the producer
and underlying contextual ledger, router and runtime reads. If a deadline or
cancellation is observed during acquisition, the error names a fixed stage
(`snapshot`, `node_status`, `operational_agents`, `runtime_classification`,
`host_health` or `projection`) and the elapsed time since that stage began.
This reports where cancellation was observed; it does not claim that stage
consumed the full acquisition budget. The producer's existing 15-second
acquisition ceiling is unchanged.

The stage report contains no task, registry, thread, host-health or database
payload. The error still unwraps to its context cause so callers can classify
`context.Canceled` and `context.DeadlineExceeded` with `errors.Is`.
If another read error races cancellation, callers can also classify that
original error with `errors.Is`; the public message remains limited to the
fixed stage and elapsed duration. The command checks cancellation again before
writing JSON, so a canceled read that returned a board does not publish it.

Runtime lane classification reuses the existing passive per-agent reads with
up to four workers. The classification helper joins its started callbacks
before returning a result, and the command refuses to publish a partial board
if classification fails or the context is canceled. A command deadline can
return the caller before the acquisition worker finishes; the active producer
operation remains occupied until that worker exits. Context cancellation stops
client-side waiting where supported, but does not retract remote work already
dispatched. The legacy non-contextual fallback may also remain active until its
call returns.

## Why is a lane UNROUTABLE?

The Fabric board (`sirsi dashboard`, `/api/fleet`) and `sirsi router fleet` show
a one-line `reason` under every UNROUTABLE lane, so a lane whose attended session
closed two days ago no longer looks the same as a broken one. The reason is read
from the same router ping result that decides routability (plus the registry and
alias table); it explains the state and never changes it. Examples:

- `retired lane (not in registry); open rows left over`
- `retired; forwards to <successor>`
- `waiting for an attended session (none seen for 2d2h)`
- `no worker configured; needs a person's session`
- `3 wake failures since the lane last acknowledged work`

`/api/fleet` also carries the ping `verdict` (LIVE, WAKEABLE, HELD, WATCH_ONLY,
and so on); the board shows it as a tooltip. Lanes that are not UNROUTABLE have no
reason. `sirsi router fleet` has no wake-failure counts, so for a lane that pings
LIVE or WAKEABLE it can only say that no automated wake path reaches it.

## Retired thread history

Thread pruning removes an exact old registration from ordinary views while
retaining its internal retirement key. Its old user payload is cleared. This
prevents a delayed heartbeat from making the old registration reappear. A
normal new session can still register under a new ID; resuming the retired ID
does not restore it. No new command or permission is required.

The correction must first reach the canonical router store owner with its
compatible-writer checks. Installing a newer desktop client alone does not
upgrade that service or prove the fix is active across the fabric.
