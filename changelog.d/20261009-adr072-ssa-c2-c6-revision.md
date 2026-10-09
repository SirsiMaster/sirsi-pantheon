- Revises ADR-072 (universal thread naming) C2/C4/C5 and the phase order per
  SSA round-1 CHANGES_REQUESTED (thr-80b0f440844c2424, 2026-10-09): C2 now
  defines the schema-hash algorithm (excludes itself from its own hash),
  requires a router-issued promotion receipt as the actual trust root (a
  client pin alone grants nothing), and states the offline carve-out and
  working-tree-vs-remote-rejection distinction explicitly. C4 separates
  idempotency (replay guard, keyed on caller+thread+op+request-digest, same
  key/different payload rejected) from concurrency serialization (revision
  CAS), and scopes the `(machine-id, session)` constraint to
  `(machine-id, session, name)` so one session's multiple live threads are
  not accidentally prohibited. C5 adds canonical/active-alias/tombstone
  states with generation-fenced reuse so a retired name's reuse cannot
  silently cross-deliver mail to an unrelated new holder, and requires
  legacy name-only sends to resolve an expected generation or be rejected as
  ambiguous. Phase order: C2/C4 are now explicit P2 preconditions, not
  P3/P5 follow-ups. No implementation lands with this change — design-only,
  pending SSA re-review before the P2 bind.
