## Fixed

- Binary-drift detection now treats distinct regular executable files as equivalent only when their retained descriptors yield the same SHA-256 and both path names still resolve to unchanged descriptors. Missing, non-regular, unreadable, changed, or unequal targets remain drift; equal path spelling keeps its existing behavior. This observes byte equivalence during the scan and makes no claim about later path or execution continuity. Version-based sibling checks remain independent.

Refs: `docs/ADR-023-BINARY-VERSION-CONTRACT.md`, PANTHEON_RULES.md A23.
