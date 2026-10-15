# Session-table churn: pre-registration (written 2026-10-15T22:31:10Z, before any code)

OBSERVED (M5, measured 2026-10-15): agent "Mac" minted 17 distinct sessions in ~30 s; the shared cache file alternated between runtime
hashes 34b5ce2b (current /Applications/Pantheon.app binary, 11 wake loops) and 8ea0cfc3 (/usr/local/libexec/sirsi-pantheon/sirsi,
8 older wake loops). A cached session is valid only for the exact binary that minted it, so two binaries sharing one cache path
invalidate each other: ~every loop tick mints. 19 loops, no SIRSI_AGENT_ID, so all authenticate as the hostname "Mac"
(audit log: "Mac@Mac" 86,449 sessions in 48 h; M1.local 4,651).

HYPOTHESES
H1 (cause): the mint rate is driven by cache flapping between runtime hashes, not by traffic volume.
   Fix A (client): the session cache path includes the runtime-hash prefix, so binaries coexist; prune cache files untouched > 7 d.
   Fix B (client): a wake loop authenticates as its own lane (SIRSI_AGENT_ID set from its argument), so loops stop sharing one
   identity (also fixes audit attribution).
   PASS: with two runtimes alternating N calls for one agent, mints == 2 (was ~N). After rollout on the M5, mints for agent Mac
   over 10 min <= 2 x distinct binaries; new "Mac@Mac" sessions/hour falls by >= 95 %.
   NEGATIVE CONTROL: with the runtime hash removed from the path again, the alternating test mints ~N.
H2 (bound): even with H1 fixed, the table needs a server-side bound. Amortized GC on mint: delete idle sessions (last_seen older
   than 24 h) and revoked sessions older than 7 d, bounded batch per call, NEVER a session referenced by an open lease or bound thread.
   PASS: rows stay under 10k steady state; GC never removes a live-leased or recently-seen session.
   NEGATIVE CONTROL: GC must refuse to delete a session whose last_seen is recent, and one referenced by a lease.
NOT CLAIMED: server table size is not directly readable by me today (no SQL path); the 86k figure is the audience-log session
count. The first server-side measurement comes from the GC's own deleted-row count after deploy.
