# Pantheon SNE Profile-Scoped Capabilities

Date: 2026-08-21

Pantheon now joins signed runtime-package capacity to the exact admitted model and runtime selection. Its catalog and governed OpenAI-compatible `/v1/models` response expose:

- cache topology,
- serving cache capacity,
- prefix-session maximum,
- derived prefix-session support.

Prefix-session support is true only when the admitted execution mode is exactly `plain` and the signed runtime reports a positive session maximum. MTP and `mtp-shared-wide` remain unsupported even if their low-level runtime package reports shared cache infrastructure. This prevents a generic cache field from being misrepresented as a product capability.

Nexus and other clients can therefore request a reusable conversation profile and receive the actual execution mode in provenance. Pantheon must reject unsupported combinations or select another explicit governed tuple; it may not silently fall back across execution mode, framework, processor, cloud, model, or precision.

Tests:

- `go test ./internal/dashboard ./internal/sne -count=1`: pass
- reciprocal policy coverage: plain plus positive capacity is supported; plain zero-capacity and all MTP modes are unsupported

Copied candidate:

`artifacts/candidates/pantheon-sne-profile-capabilities-v1-20260821/sirsi-menubar`

SHA-256: `5b50c61edad33556c6d08c04f5731d4bbc67ef6d1607a1925d3e02a54896e9a4`

The installed Pantheon and immutable releases were not modified.
