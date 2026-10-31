## Root relay refresh control

The existing root lifecycle caller now requests a PID/start and spool-root bound
acknowledgment from the active relay before retiring it. The relay pauses new
forwarding duties and waits a bounded period for admitted request/outbox work;
timeout or unresolved local settlement keeps the selected process and endpoint
alive and refuses retirement. The default `Serve` join, existing artifact pins,
service authorization and one-time 0.24.147 unknown-outcome transition remain
unchanged. Late or unread response metadata is retained until the caller fully
reads and closes it; no outcome is replayed or reconstructed.

Post-send unknown outcomes from a first attempt are parked in the existing
non-retryable `failed/` custody before the response is published. If that park
fails or an existing destination would be replaced, inflight custody remains
and sticky settlement debt blocks retirement. The transfer uses atomic
destination-exclusive rename. If a never-reached request cannot be staged in
the ordered outbox, it receives a truthful not-sent response and is retained in
failed custody; failure of that transfer leaves inflight custody and sticky
debt. Failure to publish the not-sent response also leaves sticky settlement
debt, even when the envelope reached failed custody.
Age-based sweeping preserves `res/` and `inflight/` evidence; consuming error
bytes or elapsed time does not establish remote settlement. Parking records
local custody only; it does not resolve or certify the remote outcome.
The shared spool-agent name validator is available on Windows builds as well;
its existing character and length rules are unchanged.

Refs: `docs/ADR-062-ROUTER-SERVICE-RA-CONCURRENCY.md`, `docs/ADR-069-DURABLE-ORDERED-OUTBOX.md`, `docs/stacklab/pantheon-pt/canon/ROOT_RELAY_REFRESH.md`, `docs/runbooks/LEGACY-ROOT-RELAY-MAINTENANCE.md`
