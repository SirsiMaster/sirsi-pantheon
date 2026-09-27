# Sirsi Pantheon v0.24.11 — Stack Lab release receipt

- Tag: `v0.24.11`
- Source commit: `0c6cf6a3d9cab5d0294ee7393a6a4d29bc75d520` (PR #824 merge)
- Predecessor: [`v0.24.10`](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.24.10)
- Scope: canonical router identity split for `claude-io`, `hermes`, and `hermes-m5`
- Stack Lab boundary: Hermes software owns the M1 Photon seat; `hermes-m5`
  owns the M5 transport seat; `claude-io` remains the I/O pillar.

## Verification

- PR #824 passed binding hold, lint, tests, secrets scan, and ARM64 build.
- The release is an identity/configuration correction; it does not claim
  signing, notarization, cask publication, or installed-host lifecycle proof.
