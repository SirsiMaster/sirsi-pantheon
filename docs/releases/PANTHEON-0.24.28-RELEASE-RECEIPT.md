# Sirsi Pantheon v0.24.28 release receipt

Date: 2026-09-28

This release makes the macOS DMG the commercial baseline. The tagged workflow
still fails closed when Developer ID Application or Apple notarization inputs
are missing, builds the canonical Swift menubar and Go engine into one
Pantheon.app, signs/notarizes/staples/validates that DMG, and publishes its
exact bytes for the canonical cask. A PKG is additive and is built, signed,
notarized, stapled, validated, and uploaded only when the separately managed
Developer ID Installer identity is present.

## Stack Lab

- Wing: `stacklab.wing.pantheon-pt`.
- Recipe: `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`.
- The recipe records the DMG-first boundary and the optional PKG credential.
- The tagged release becomes the canonical source starting point for the next
  Pantheon candidate after exact artifact readback.

## Boundaries

This source receipt does not claim a PKG unless the tagged workflow publishes
one, and it does not claim installed-host lifecycle qualification. Those claims
require exact workflow artifact readback and separate host evidence.
