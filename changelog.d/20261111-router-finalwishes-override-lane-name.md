- **Router recipient admission: seven-day FinalWishes override, keyed on lane name.**
  Owner decision 2026-11-11: until 2026-11-18T05:50:00Z, `claude-finalwishes-m1`,
  `claude-finalwishes-m5`, `codex-finalwishes` and `sirsi-software-admin` may send to one another
  past the recipient limit (both ends must be in the set; the web lanes are not). The sender name is
  client-supplied and bound to the caller only for lane-scoped tokens, which the Macs do not use; SSA
  (item 20261111-064341) asked for verified identity, and the owner, shown that a verified-only rule
  would admit nothing, accepted name-keyed matching for the window. Exposure: a lane on the same hosts
  can assert one of the four names and send past the limit to those four lanes, nothing else. Applies
  to `SendGuarded` only (raw `Send` keeps the limit). Each over-limit admit bumps
  `finalwishes_override_admits` and a per-item `finalwishes_override_admit:<item id>` counter in the
  item's own transaction. Delete the block in `internal/routerstore/facade.go` after 2026-11-18.
  Refs: PANTHEON_RULES A23, A34.
