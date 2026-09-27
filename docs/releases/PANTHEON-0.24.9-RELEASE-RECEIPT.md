# Sirsi Pantheon v0.24.9 release receipt

## Source

- Release purpose: self-hosted macOS signing cleanup from PR #820.
- Product source after merge: `331113d8` (full commit recorded by the release tag).
- Version: `0.24.9`.
- Protected PR #820 checks: binding-hold, lint, secrets, tests, and arm64 build passed.

## Artifacts

| Artifact | SHA-256 | Size |
|---|---|---:|
| `bin/SirsiPantheon-0.24.9-arm64.dmg` | `cd0a6fb573bb1fa3c3ce7fdb8c030d184ec8bb5b2ee3d313bed004e1b9ea0d3c` | 11,996,167 bytes |
| `bin/SirsiPantheon-0.24.9-arm64.pkg` | `47a09015b562521d5f9b78f4d42532a1a7eaabd16060249e69fc49ab53d03736` | 10,643,828 bytes |

The local arm64 release candidates remain ad-hoc/unsigned because no Developer
ID Application or Installer identity is configured. This receipt makes no
notarization, stapling, cask, or installed-host claim.

## Change

The signing job now restores `login.keychain-db` as the default keychain,
restores the login+System search list, and deletes its temporary
`build.keychain` in an `if: always()` cleanup step. The cleanup runs after
success, failure, or skip, preventing persistent self-hosted-runner keychain
hijacking and repeated assistantd prompts.

## Remaining commercial evidence

Developer ID signing, notarization/stapling, cask publication, and installed-
host upgrade/rollback/uninstall evidence remain open.
