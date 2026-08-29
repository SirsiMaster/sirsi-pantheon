# Pantheon Installed Identity and Duplicate Cleanup

Date: 2026-08-22

## Installed control surface

- Active application: `/Applications/Pantheon.app`
- Bundle identifier: `ai.sirsi.pantheon`
- Version: `0.23.9-beta`
- Running process: `/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar`
- Current signature: ad hoc (`TeamIdentifier=not set`)
- Homebrew ownership: none

The active installation was deliberately not replaced. An ad-hoc successor
would churn macOS TCC identity and recreate permission prompts.

## Visible Login-keychain signing state

The visible Login keychain contains valid Apple Development and Apple
Distribution identities for team `9D382WV988`. It does not currently contain a
Developer ID Application identity with its private key. Apple Distribution is
not a substitute for Developer ID Application and must not be used to label a
direct-download package as notarizable Developer ID software.

Release packaging remains fail-closed until the Developer ID Application
certificate and private key are present in the visible Login keychain. Pantheon
does not search hidden keychains, and a sudo password is not represented as a
Developer ID private-key credential.

## Duplicate cleanup

Two inactive bundles with the same Pantheon identity were moved, recoverably,
to:

`~/.Trash/Pantheon-duplicate-cleanup-20260822-110249/`

- `~/Applications/Pantheon.app` (`0.20.0`, Developer-ID signed, obsolete)
- `~/Applications/Sirsi Menubar.app` (`1.0`, ad-hoc, obsolete)

The active `/Applications/Pantheon.app` process remained running throughout.

## Prevention

- `scripts/verify-menubar-release-contract.sh` enforces the canonical release
  entrypoint and permission-silent resident projection contract.
- `scripts/build-dmg.sh` fails closed when release signing is required but the
  visible Login keychain lacks the requested Developer ID identity.
- `docs/RELEASE_SIGNING.md` no longer claims that the SwiftUI prototype may
  silently replace the canonical Go control engine.

