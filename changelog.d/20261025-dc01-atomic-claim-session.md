- Commit authenticated inbox claims and their session ownership in the same
  store transaction (DC01). A failed owner-binding write rolls back the item
  lease and claim counter instead of acknowledging a usable unbound claim.
  Existing local/unclaimed and delegated close compatibility is unchanged.
  Source qualification and installed backend adoption are separate evidence.

Refs: docs/ADR-068-ROUTER-A2A-MCP-FRONT-END.md,
internal/routerstore/README.md.
