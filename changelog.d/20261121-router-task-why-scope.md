### Fixed

- Scope task eligibility diagnostics to their existing task-row predicates with
  explicit JSON scope/caller-not-evaluated metadata. Remove the unconditional
  claim-success promise; plain-text `router task why` warns when an older service
  lacks scope metadata. Row claimability/dispatchability and all authorization
  guards are unchanged. Current-main composition preserves evidence-bound retry
  recovery guidance; its tests are unrun. Not an installed fix or a substitute
  for the separately documented M5 caller-binding operational recovery.
