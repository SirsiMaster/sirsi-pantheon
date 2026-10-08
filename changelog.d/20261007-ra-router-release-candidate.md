## Ra one-authority router release candidate

- Refuses implicit production use of `~/.sirsi/router.db`; service resolution is
  fail-closed, while explicit isolated SQLite remains available for tests and
  migration.
- Makes Ma'at's test ledger binding explicit so tests cannot inherit the live
  router authority.
- Publishes the complete Ra router architecture, release contract, traceability
  matrix, threat model, runbook, and Stack Lab recipe in the Ra wing.
- Adds a hermetic one-authority negative control and preserves the existing
  routerstore, dispatch, authentication, relay, A2A/MCP, wake and rollback
  contracts.
- Classification: platform-foundation/pilot. Fresh cloud readback, third-machine
  rehearsal, full fleet parity, and commercial GA remain explicitly unclaimed
  where the releasing identity lacks current receipts.
