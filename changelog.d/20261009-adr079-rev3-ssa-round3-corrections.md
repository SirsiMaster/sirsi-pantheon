- docs(fleet-safety): ADR-079 rev3 — corrects 4 further gaps SSA's round-3
  review found (head `5db59a8e`): disclosed-but-nonblocking coverage gap
  now requires registry-union-independent-discovery reconciliation that
  blocks readiness; QUIESCENT invalidation moves from self-report + a
  pre-action re-check to per-participant quiescence adapters plus an
  executor-held atomic generation-bound fence (closing the check-to-act
  race); saved-artifact "identity" now requires content hash+size or a
  qualified adapter receipt plus confirmed durable publication, with
  Thoth's own durability a hard blocking dependency where relied on;
  `success` reframed as one of two jointly necessary conditions with
  execution authorization, and a replayed terminal record is defined to
  return only the historical decision, never re-establish readiness.
  Status stays Proposed.
