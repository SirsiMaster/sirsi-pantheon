# Sirsi Pantheon v0.24.8 release receipt

## Source

- Release purpose: fail-closed router delivery-boundary hardening from PR #814.
- Product source after merge: `79aae61b` (full commit recorded by the release tag).
- Version: `0.24.8`.
- Local verification: `go test -race -short ./internal/routerstore/...`, `go vet ./internal/routerstore/... ./internal/maat/... ./internal/guard/...`, and CLI/agent builds passed.
- Protected GitHub CI for PR #814: all reported checks passed, including binding-hold, docs canon, lint, secrets, tests, and arm64 build.

## Artifacts

| Artifact | SHA-256 | Size |
|---|---|---:|
| `bin/SirsiPantheon-0.24.8-arm64.dmg` | `62be35f847b23c89e188e73ef14d51568211c4f84239a7be95cca8c02854aa2f` | 11,995,820 bytes |
| `bin/SirsiPantheon-0.24.8-arm64.pkg` | `994b8e760696e9a5f49e99898c886399db92a160bb4bab2f534796129703071f` | 10,643,826 bytes |

The DMG and PKG are reproducible local arm64 release candidates. They are
ad-hoc/unsigned because no Developer ID Application or Installer identity is
configured; they are not represented as notarized or distributable builds.

## Change

The keystone now fails closed if home-directory marker inspection errors. The
outbox stops at the first non-delivery frontier, preserves post-send
`OUTCOME-UNKNOWN` records in `failed/`, and only treats a dial-phase network
error as never-reached. These boundaries are covered by negative-controlled
regression tests in PR #814.

## Remaining commercial evidence

Developer ID signing, notarization/stapling, cask publication, and installed-
host upgrade/rollback/uninstall evidence remain open. The next build must use
this tagged source and replace the local release candidates with signed bytes.
