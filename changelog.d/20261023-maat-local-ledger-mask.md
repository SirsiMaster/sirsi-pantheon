### Fixed

- Selectively port reviewed 3fc33193 declarations to the twelve existing named
  restricted consumers: add only the exact Ma’at decision journal, mutation lock
  and supervisor evidence directory beside the existing token-free relay. Keep
  current identity, grants and network restrictions unchanged. Cover every named
  declaration and exact home rebasing; broad/sibling filesystem declarations fail
  the source regression. No live worker mask or policy is changed. Parents remain
  custodian-provisioned; actual future-worker append/evidence and sibling-denial
  qualification remains separate from source tests.

Refs: scripts/maat/README.md; PANTHEON_RULES.md A11/A16/A35; ADR-062;
maat-local-ledger-mask-20261018.
