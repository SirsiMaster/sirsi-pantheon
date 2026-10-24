### Fixed

- Horus lane-delivery alerts no longer invent `wake:none`, an empty thread
  registry or a unique repair from an unavailable delivery path. Single-lane
  and fleet notices distinguish the observed lack of a fresh usable thread
  from unknown configuration, stale registrations and actual queue delivery.
  Backlog totals, classification, active-thread suppression and stable dedup
  titles are unchanged. Regression fixtures cover both sides of the rollup
  threshold and demonstrate the old producer's semantic failure.

Refs: ADR-061, PANTHEON_RULES.md A35, docs/QA_PLAN.md.
