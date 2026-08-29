# SNE Signed Runtime Catalog Gate

Date: 2026-08-18

## Result

Pantheon now verifies an exact detached Ed25519 signature before parsing or
trusting a runtime catalog. Production default lifecycle configuration requires
the signature and a pinned public-key path.

The installed catalog passed both the low-level verifier and Pantheon's default
lifecycle loader:

- catalog ID: `sne-gemma4-v1`
- entries: `11`
- catalog SHA-256: `1975d968070293a8476e1b0236e69a2f6fb4643b8e1cec94ffcba5d6850cedf2`
- signature-file SHA-256: `b72cef6ba188be3b094ec377afc4b299bf579403ac1afb83bacf76c6b9888214`
- public-key-file SHA-256: `60a94a48404769b133589e9b23ba137a778789b037c36af48e5f4fb6cf8b9808`
- private-key permissions: owner-only `0600`

The private key is stored outside source and runtime packages. The public trust
root is safe to distribute. The signer refuses identity overwrite, writes files
atomically, and signs the exact catalog bytes. Tests reject a one-byte mutation,
wrong trust root, malformed signature, non-Ed25519 key, and ambiguous runtime
selection.

## Executable evidence

- `go test ./cmd/sirsi-sne-catalog-sign ./internal/sne ./internal/dashboard`
- `TestRealSignedRuntimePackageCatalog`
- `TestRealDefaultSNELifecycleLoadsSignedCatalog`

## Distribution boundary

This proves the installed M5 catalog. Its entries still contain host-absolute
package roots, so the same signed bytes are not a clean-host release catalog.
Release work must separate a portable signed package manifest from the local
materialized path projection, or otherwise produce and authenticate the local
projection without shipping the private signing key. Do not claim clean-host
catalog portability until that gate passes on M1 and M5.

## Portable materialization follow-on

Pantheon now supports signed release entries containing `package_id` rather than a
host-absolute `package_root`. Signature verification occurs first. Pantheon then
materializes the ID beneath its configured packages directory and retains the
portable ID in lifecycle state. IDs containing traversal, path separators,
spaces, or excess length fail; entries containing both identity forms fail; and
different runtime selections cannot share one materialized package root.

Focused suites and the real legacy installed-catalog gate passed after this
change. A clean-host release claim still requires generating the portable signed
catalog and proving identical signed bytes on M1 and M5.
