# Pantheon v0.24.10 — Stack Lab release receipt

- Tag: `v0.24.10` (pending merge/tag publication)
- Feature source: `098bfd95f00c39d348ca9280b4df1b4c2702d8a9`
- Feature PR: [#803](https://github.com/SirsiMaster/sirsi-pantheon/pull/803) (merged)
- Predecessor: [`v0.24.9`](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.24.9)
- DMG SHA-256: `7385be30aa18b861401cd033007a868cd48688d56c6c439d61cbffebf9cce8a2` (11,996,267 bytes)
- PKG SHA-256: `533fd43747121ceefba7765a9e5b645552087239ae4aa59b1caed02763c9e75b` (10,643,931 bytes)

This release maps the stable `io-connect` wing id to the renamed
`sirsi-hermes` transport repository and adds the `sirsi-photon` hardware
repository. The local router identity hook follows the same split, preventing
Stack Lab doctor and session identity resolution from using the retired repo
name.

## Verification

- PR #803 checks passed: binding hold, lint, secrets scan, tests, and arm64 build.
- Local stacklab tests, vet, CLI/agent build, DMG/PKG build, and diff checks passed.
- This release is a repository identity/metadata correction; no signing,
  notarization, cask, or installed-host lifecycle claim is made.
