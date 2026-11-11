- **Router recipient admission: seven-day FinalWishes override, bound to verified caller identity.**
  Owner decision 2026-11-11: until 2026-11-18T05:50:00Z, `claude-finalwishes-m1`,
  `claude-finalwishes-m5`, `codex-finalwishes` and `sirsi-software-admin` may send to one another
  past the recipient limit (both ends must be in the set; the web lanes are not). The exemption
  applies only to a VERIFIED sender: a `SendGuarded` call the service authenticated with a
  lane-scoped token whose scope contains the sender. Bootstrap and unscoped host tokens, relays,
  local callers and the raw `Send` path keep ordinary saturation handling; no token gains any
  authority and the wire format is unchanged. Each over-limit admit bumps `finalwishes_override_admits`
  and a per-item `finalwishes_override_admit:<item id>` counter in the item's own transaction.
  Delete the block in `internal/routerstore/facade.go` after 2026-11-18.
  Refs: PANTHEON_RULES A23, A34.
