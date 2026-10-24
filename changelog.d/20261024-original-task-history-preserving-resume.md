Fix the supported-operation gap for exhausted original ledger rows: add a
typed own-lane inspection and explicit evidence/row-snapshot-CAS recovery
claim. One fresh normal lease increments attempts without resetting failure
history, binds the authenticated worker atomically, and records immutable
provenance. Live/malformed custody, foreign identities, unresolved blockers,
stale snapshots and changed replays refuse. Completion still requires the
existing authenticated live lease; there is no direct DONE route.

No task schema, old-client claim ceiling or generic task update contract
changes. The new CLI refuses a local store and requires the compatible
configured canonical service/relay. Evidence hashes are actor attestations,
not service verification of client files. SQLite/PG, real RPC and CLI fixtures
are prepared, not executed; no existing original task or live service changed.
