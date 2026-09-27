# Security and threat model — Ma'at

Threats include forged or incomplete evidence, unauthorized journal writes, Casebook projection becoming an authority, stale host state, and cross-lane scope confusion. Controls are append-only records, explicit evidence links, deterministic read-only projection, strict status validation, local write-boundary ownership, and fail-closed missing input. Secrets, credentials, model endpoints, and remote policy stores are out of scope for this lane.
