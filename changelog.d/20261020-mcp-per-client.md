### Fixed

- Allow independent IDE sessions to use `sirsi mcp` concurrently instead of
  exiting behind a machine-wide singleton. Keep legacy `sirsi seshat mcp` on
  the same clean JSON-RPC transport and preserve shared-store locking.
