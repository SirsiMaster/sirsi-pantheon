# Sirsi Pantheon 0.24.23 Release Receipt

Date: 2026-09-28

## Source identity

- Release source: `479757bae877ab32b7c67e59434d2fadd5a0f6f5` (merged PR #843).
- Source tree: `d32470ded389f96cf2aae5f77643280bb111a6aa`.
- Predecessor: `v0.24.22`.
- Change: `sirsi stacklab doctor` reads the Stack Lab wing roster from the
  canonical `sirsi-pantheon` `origin/main` remote reader instead of a stale
  local working-tree roster; peer resolution uses the same reader.

## Verification boundary

- Protected main CI: run `36383610638` completed successfully for the exact
  source commit, including lint, tests, and both ARM64 build jobs.
- Gitleaks run `36383610695` completed successfully.
- The tag workflow is the authority for static CLI assets and GoReleaser
  checksums. macOS Developer ID signing, notarization, Homebrew cask
  publication, and installed-host lifecycle evidence are reported separately
  by that workflow and are not inferred from this source receipt.
- No live router migration, service mutation, credential change, or installed
  host qualification is claimed by this receipt.

## Stack Lab

This receipt is the Pantheon PT wing source package for `v0.24.23`. After the
release workflow completes, its exact tag, asset checksums, and any open
signing/cask boundary become the canonical starting point for the next
Pantheon candidate.
