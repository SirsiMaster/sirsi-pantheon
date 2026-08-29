# Pantheon SNE Dual Runtime Identity Closure

Date: 2026-08-21

Pantheon now treats the SNE service executable and loaded native inference engine as different governed artifacts. `runtime_sha256` identifies `bin/sned`; `native_runtime_sha256` identifies the loaded `libsirsi_native_runtime.dylib`; and `native_runtime_dylib` is the authorized package-contained path. Status and readiness must return both expected hashes.

Focused tests passed for `internal/sne`, `internal/dashboard`, and `cmd/sirsi-sne-supervisor`.

Copied candidate: `sne-v2-mtp-shared-wide-api4096-security-v3-candidate`.

- Service: `749d6cd6bb5c9532431f93f3f81f87449e2c0ee10d3eaae0bcb6e092e0e72cab`
- Native: `15a3b4b975e8191df0e97f890060ebfe78b5a3dd7bc2dfbc2baaefb560c2e115`
- Manifest: `c3e45a153aca6edb11d3176d33d7c4d197969cede3902e2afe25e5b268bd1443`
- Checksum set: `598369a036177c059db5765c9f695deca875debee005b16d2cf04e443fdfbf28`

This proves identity composition and fail-closed lifecycle behavior, not live Metal/API4096 correctness or performance. Permanent rule: launchers, supervisors, receipts, and provenance preserve both identities; a generic runtime hash is inadmissible.
