# Sirsi Pantheon v0.24.24 release receipt

Date: 2026-09-28

This patch release clarifies the router operator contract for `--blocked-by`.
The flag is an external-block reason, matching the store and live task records;
it is not a dependency-task identifier. No router migration or live service
mutation is claimed by this source release.

## Exact source and validation

- Release candidate source: `55facc7fffaf457f0b2f009e5499b950ecb6b321` / tree `d4db32f08d052f29834430b4be3b12e40fc916c0`.
- Base release: `v0.24.23`.
- The candidate incorporates the rebased RS-39 help and continuation documentation
  from the superseded PR #759 branch.
- CI, GoReleaser, signing, notarization, cask, and package readback remain the
  authority for the published release artifacts.

## Boundaries

This receipt does not claim router migration, live service deployment, Windows
packaging, or installed-host lifecycle qualification.
