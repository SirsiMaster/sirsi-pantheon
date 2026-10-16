- feat(fleet-safety): ADR-079 P1 — `internal/maintenance`: `EnumerateRegistered`
  reads the live thread registry for a scoped host (excludes terminal and
  suspended threads), and `Mint` issues an expiring, digest-bound provisional
  `Txn` over that list. Strictly preservation-only per SSA's
  ACCEPT_BOUNDED_DESIGN (PR #1055 head `6fe284ce`): `Txn.Provisional` is
  always `true`, and nothing in this package can grant readiness/`QUIESCENT`
  — the reconciled registry-union-independent-discovery mint and the
  quiescence/fence machinery are P2/P3 work, not built here.

  Refs: PANTHEON_RULES.md §2.29/A32, docs/ADR-079-SAVE-BEFORE-MAINTENANCE-CONTRACT.md (PR #1055)
  Changelog: v0.24.x — maintenance P1 enumerate+mint
