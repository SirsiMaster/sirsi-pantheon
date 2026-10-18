### Fixed

- Candidate: fence Ma’at reservation updates with canonical-store CAS, rederive
  conflicts, and keep expiry reads from overwriting concurrent grants. Reject
  legacy unfenced writes, retain history and future fields, and distinguish
  same-holder tickets created in the same second. Server-first rollout and
  independent deployed qualification remain required (ADR-062 amendment).
