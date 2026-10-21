### Fixed

- Router completion-proof validation now binds evidence to the Git worktree that
  invoked the close while dispatch continues to use the canonical router
  service. CLI and router-MCP closes reject unknown callers and proofs from a
  different repository instead of borrowing Pantheon's contract.
