# Sirsi Pantheon 0.24.14 Release Receipt

Date: 2026-09-27

## Source identity

- Release source: `bc438de93ff90d0c6fd94287c30cba5e971db867` (merged PR #829)
- Predecessor: `v0.24.13`
- Release branch: `release/v0.24.14`
- Change: add the canonical `cylton-hermes` router identity for the Hermes M5
  `sirsi-io-connect` seat, with no service or credential mutation claim.

## Verification boundary

The candidate must pass the protected Pantheon build, test, lint, secrets, and
binding checks before tag creation. The release workflow is authoritative for
published CLI assets and their checksums. macOS Developer ID signing,
notarization, cask publication, and installed-host lifecycle evidence remain
separate gates and are not claimed by this source receipt.

## Stack Lab

The release is submitted to the Pantheon Stack Lab wing as the canonical
starting point for the next candidate. The corresponding Stack Lab receipt
must bind this tag, source commit, release assets, and the open packaging
boundary.
