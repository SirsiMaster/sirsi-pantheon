### Test — cover bounded GitHub MCP envelope framing

- Add inert `create_blob` and `create_tree` hook-envelope fixtures at exact
  64 KiB boundaries and larger representative sizes, plus short-chunk,
  truncation, invalid-UTF-8, and trailing-data reader cases. Oversized inputs
  must retain the adapter's explicit cap error; the production limit and deny
  behavior are unchanged.

Refs: `cmd/sirsi-pretoolguard/README.md`,
`docs/ADR-119-SLEEP-CUSTODY-PRODUCTION-CONTROLLER.md`
