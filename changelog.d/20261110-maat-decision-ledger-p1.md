- **feat(router):** Ma'at decision ledger P1. Schema v26 adds the append-only
  `maat_decisions` table (id = full sha256 of the raw bytes), and
  `IngestDecision` dedups identical bytes. `IngestDecision` is not served over
  RPC until dispatch stamps producer host and session from the authenticated
  session. The Postgres schema mirror, spool tailer and CLI read path are
  separate follow-ups.
