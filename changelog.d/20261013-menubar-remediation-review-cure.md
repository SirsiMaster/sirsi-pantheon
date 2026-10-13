### Fixed
- Bind native repair confirmation to immutable finding evidence and mapped command; revoke changed, cleared or stale approvals before launch.
- Bound native command capture even when a child ignores SIGTERM or a descendant keeps a pipe open; preserve partial output and report timeout without a success exit status.

Refs: SAFETY_DESIGN.md, docs/ADR-033-REMEDIATION-CATALOG.md
