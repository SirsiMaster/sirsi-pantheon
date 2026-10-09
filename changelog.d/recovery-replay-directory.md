# Unreleased — recovery replay directory hardening

Reject replay directories not owned by the bridge effective operator or not mode
0700, both at startup and before each create-only claim. Retained-descriptor checks
fail closed on stat errors and permission drift. No issuer/enrollment, ingress,
release install or Apple input-release authority is supplied.

Refs: PANTHEON_RULES.md A11/A16/A35; ADR-075 (proposed authority; no approval implied).
