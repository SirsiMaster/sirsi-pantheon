# Apollo lifecycle and admission

Pantheon supervises Apollo through the existing SNE protocol/package names.
Live resource admission protects foreground memory. An enrolled profile also
pins hardware_observation (local SHA v2 JSON file), hardware_node_id (exact host
key, such as M1), and hardware_collector_sha256. All three are required together.
Supervisor checks this independent receipt before every launch/restart, then
runs the existing live memory checks. It requires the exact node's successful
capture and every qualification component to PASS; stale (five minutes), future,
missing, unreadable, mismatched, HOLD and UNKNOWN receipts refuse launch.

The local receipt path/config must remain under operator control. Collector hash
is a provenance pin, not a signature or authentication of remotely supplied JSON.
Receipt publication must be atomic. PASS_OBSERVATION_ONLY is an additional gate,
never a substitute for live memory/footprint admission. Unenrolled profiles keep
their existing live checks; fleet enrollment is explicit. Desktop recovery does
not call this gate. No processes, protection settings or hardware are modified by
reading observations. Tests inject file reads; supervisor rejection is tested
before any process launch. See ADR-064 for remote recovery authority.
