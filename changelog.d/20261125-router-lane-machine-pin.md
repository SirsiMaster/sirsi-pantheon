### Added
- The router now keeps its own server-side record of which Mac each lane belongs to (ADR-065 Decision 6, input for
  rs-37; owner decision 2026-11-25). Schema v30 adds `lane_machine_pins(agent, machine_id, set_by, reason, created,
  retired)`: a lane may hold several active pins, a pin is retired and never deleted (history kept), and nothing is
  backfilled. The pin is set only by the router operator (`sirsi router lane-pin set|retire|list --store <backend>`,
  the same surface as `router token mint|revoke`); `SetLanePin`, `RetireLanePin`, `ListLanePins` and
  `ActiveLanePinMachines` are not served over the wire and are refused by the relay, so a lane cannot set, retire or
  read a pin. It is never derived from a thread's self-asserted host.
- **Report-only.** At a served `Wait` the service compares the caller token's adopted machine id
  (`host_tokens.machine_id`) with the lane's active pins and counts `match`, `mismatch`, `unpinned` or
  `no-machine-id` in bounded in-memory counters (per pinned lane per verdict; every unpinned lane shares one label;
  hard key cap), summarized in the service log every 5 minutes. No request outcome changes for any verdict, and a
  failed or panicking evaluation is counted and ignored. The one enforcement seam is `lanePinRefuses`, which returns
  false; there is no switch to turn enforcement on.
- Cost: one indexed read per lane per 60 s per service instance (10 s after a failed read); a pin change is seen by a
  running instance within 60 s.
- Deploy order: the router service and every `sirsi` binary that opens the Postgres ledger require schema version 30
  exactly. Apply `pg/schema.sql` (the apply-schema job now expects 20 tables and version 30), then roll the service,
  the token job and the Mac binaries to the new image together; a SQLite store upgraded by a v30 binary is refused by
  older binaries. Until the operator pins lanes and each Mac token has adopted its machine id, every verdict reads
  `unpinned` or `no-machine-id`.

Refs: PANTHEON_RULES.md A35, ADR-065 Decision 6, docs/design/ADR065_INFORMER_AUTHORIZATION_DECISION.md §8
