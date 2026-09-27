# Sirsi Pantheon v0.24.13 — Stack Lab release receipt

- Tag: `v0.24.13`
- Source commit: `e4b51136` (PR #827 merge)
- Predecessor: [`v0.24.12`](https://github.com/SirsiMaster/sirsi-pantheon/releases/tag/v0.24.12)
- Scope: router deployment pins the `sirsi-router-token` Cloud Run job to the
  image used by the just-deployed `sirsi-router` service.
- Stack Lab wing: `router-service` / A2A control plane.

## Verification boundary

- PR #827 passed the ARM64 build and runtime smoke, test suite, PostgreSQL
  router-store checks, Ma'at Pulse, lint, ADR-number, direct-open, menubar,
  and Gemma resolver guards.
- This receipt covers source and CLI release packaging only. Developer ID
  signing, notarization, DMG/PKG publication, cask lifecycle, and installed
  host evidence remain separate gates. No live Cloud Run deployment is
  claimed by this source release receipt.
