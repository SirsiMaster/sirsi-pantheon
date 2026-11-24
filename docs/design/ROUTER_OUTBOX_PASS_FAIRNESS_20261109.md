# Relay outbox pass fairness — source-only successor

Classification: platform-foundation/internal operator tooling. User: existing
relay clients; pain: a live lane worker can spend an arbitrary successful held
prefix without rescanning fresh req. Ra owns router semantics; SSA owns this
source candidate/review handoff. Source controls are UNRUN, not a product release.
Refs: ADR-069 ordered outbox; ADR-062 relay; A35 scope check to actual claim.

The real lane worker now attempts at most ONE ordered held retry after each
normal fresh-request dispatch pass. A positive delivery with untouched tail
immediately continues to a normal req rescan. A held/unknown/local-failure
frontier instead retains the existing backed-off wake. Empty outbox waits as
before. No new goroutine/timer, persistent cursor, RPC/schema/auth/transport
option or altered semaphore/lifecycle ownership is introduced.

The unlimited drainOutbox wrapper and serveOnce contract remain. Retry custody,
never-reached classification, unknown parking, response publication, existing
legacy queued-read behavior, caller202 responses and cancellation/drain debt
remain unchanged. No automatic response overwrite or new replay allowance.
This does NOT compose the independent queued403 or b33 shared-read successors.

It bounds invocations per pass, not duration/HTTP attempts or scheduler/FS/CPU.
One held forward, semaphore wait or directory scan can still exceed30s. No
hard pickup SLA, global fresh/held FIFO or actual incident cause is asserted.
No current queue/host/installed binary/runtime was queried or repaired.

Three authored control declarations use private spool files/channel-gated fake
RoundTrippers, no listener/service/token/reservation. The same-lane causal case
requires a new MaatRead to settle while the SECOND held forward is still gated,
preserving held001/002/003 order and single-outbox concurrency. Reverting ONLY
the worker quantum1 to0 must restore the old starvation/watchdog failure; the
separate helper budget/unlimited/custody controls stay the same. That RED and
candidate GREEN are EXPECTATIONS, not executed results or syntax qualification.
After positive yield, actual-worker transition controls also require return to
the normal wake wait after held/unknown/cleanup failure/empty/pause, and exit on
cancellation. An unbuffered wake handshake detects a stale continuation flag;
removing only the per-pass reset is a second unexecuted expected-RED control.

Plan: assessment/custody COMPLETE; source/controls COMPLETE; independent Boole
review PENDING; build/normal/race/native/source-specific admission/qualification
UNRUN. Existing parent obligations and all safety/release/G1/G2 holds remain.
Private source snapshot only, no origin publication or Desktop/Workspace mirror
claimed. Not whole-app completion; parent owns later qualification/integration.
