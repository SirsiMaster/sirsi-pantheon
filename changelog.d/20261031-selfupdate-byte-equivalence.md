## Fixed

- On macOS and Linux, binary-drift detection treats distinct regular executable files as equivalent only when bounded reads from retained descriptors yield the same SHA-256 and both names still resolve to unchanged device/inode/mode/owner/link-count/size/mtime/ctime metadata. Nonblocking opens reject special-file substitutions before reading. Missing, non-regular, unreadable, changed, oversized, or unequal targets remain drift; equal path spelling keeps its existing behavior. This observes byte equivalence during the scan and makes no claim about later path or execution continuity. Version-based sibling checks remain independent.

Refs: `docs/ADR-023-BINARY-VERSION-CONTRACT.md`, PANTHEON_RULES.md A23.

- Align the cancellation fixture diagnostic with the repository spelling rule; its assertion and runtime behavior are unchanged.
