### Fixed

- Stage the commercial DMG/PKG pair in a private release draft, reverify the
  existing Developer ID/staple boundaries and exact remote SHA256 bytes, then
  publish. Published retries are read-only exact-byte checks; partial or
  mismatched public records refuse without asset overwrite. Preserve canonical
  Go cask/build/signing authority and always-run keychain cleanup. Add inert
  fake-gh regressions and a retained per-attempt verification receipt.
- Source-only corrective checkpoint; no actual release, signing, notary,
  installation, tests or build execution is asserted by this fragment.
