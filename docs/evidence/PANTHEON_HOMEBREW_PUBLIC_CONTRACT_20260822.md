# Pantheon Homebrew Public Contract Evidence

**Date:** 2026-08-22
**Commit:** `homebrew-tools@1ea137f`
**Disposition:** source and public resolution repaired; clean-host installation remains open

## Defect

The tap exposed both a cask and formula under the token `sirsi-pantheon`.
Homebrew guessed the formula, emitted cask/formula warnings, and then told the
user that the package was not installed. The old cask also used obsolete
LaunchAgent copy/load behavior and documented the nonexistent `sirsi doctor`.

## Repair

- `sirsi-pantheon` is exclusively the Pantheon Mac application cask.
- `sirsi-pantheon-cli` is exclusively the headless CLI/agent formula.
- The cask exposes the CLI bundled inside `Pantheon.app`.
- Login registration routes through `sirsi surface install gui`.
- Diagnosis routes through `sirsi diagnose`.
- The package contract is recorded in `PANTHEON_PACKAGE_CONTRACT.md`.

The isolated commit was pushed to `SirsiMaster/homebrew-tools` main. The
pre-push secret gate passed.

## Public resolution proof

After `brew update`:

- `brew info --cask sirsimaster/tools/sirsi-pantheon` resolved only the app
  cask, version `0.23.8-beta`, with `Pantheon.app` and its bundled CLI.
- `brew info --formula sirsimaster/tools/sirsi-pantheon-cli` resolved only
  the headless formula.
- `brew search sirsi-pantheon` displayed the two explicit identities.
- The published DMG SHA-256 in GitHub release metadata exactly matched the cask:
  `113684229f1d866c1edd267af620455dc9e5d655fd774b5145e6d1413031e221`.

## Clean-host finding

The authorized M1 target has no Homebrew in any standard location and no
Pantheon application. This proves Homebrew cannot be the sole first-run path.
The signed/notarized DMG must remain the primary clean-user installer and
Homebrew an optional distribution route. The checksum-pinned DMG installation
was not executed because the M1 became unreachable over SSH before download;
no remote bytes changed. Clean-host install, Gatekeeper, upgrade, rollback, and
uninstall evidence remain open under closure item 4.

