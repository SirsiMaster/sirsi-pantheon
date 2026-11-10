- **fix(router):** the store-migration manifest now copies `host_token_scopes`.
  Without it, a copied scoped token read as unscoped. A test now fails on any
  table left out without a stated reason. `MintScopedHostToken` now writes a
  token and its scopes in one transaction, so no reader or backup can catch the
  token without its scopes. Adds `docs/router/ROUTER_RECOVERY_PLAN.md`: state
  inventory, a live backup and point-in-time-recovery readback, the conditions
  for a complete recovery point, an inactive-clone restore procedure, and the
  token-scope plan.
