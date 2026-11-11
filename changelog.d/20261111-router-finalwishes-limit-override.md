- **Router recipient admission: seven-day FinalWishes override.** Owner decision
  2026-11-11: until 2026-11-18T05:50:00Z, a send is exempt from the recipient-saturation
  refusal iff BOTH sender and recipient are one of `claude-finalwishes-m1`,
  `claude-finalwishes-m5`, `codex-finalwishes`, `sirsi-software-admin` (the two web lanes
  are not included). Applies identically on `SendGuarded` (CLI send and respond) and raw
  `Send`. Quota, breaker, idempotency, caller binding and audit rows are unchanged; each
  admission past a saturated recipient bumps the `finalwishes_override_admits` counter.
  Delete the block in `internal/routerstore/facade.go` after 2026-11-18.
  Refs: PANTHEON_RULES A23.
