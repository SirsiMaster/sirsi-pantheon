Route the completion entry fixture through the canonical ADR-062 Store resolver
instead of a direct database constructor. Keep its HOME/service environment
isolated and explicit owned SQLite path; assert the fixture backend and regular
DB file. Preserve all CLI/MCP completion status, lease and proof assertions.
The direct-open gate remains armed and unchanged, with no allowlist exemption.
