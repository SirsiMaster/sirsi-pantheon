# Sirsi Pantheon v0.24.28 release qualification receipt

Date: 2026-09-28

This source record defines the release qualification route; it is not a
commercial release receipt. The tagged workflow fails closed when Developer ID
Application, Developer ID Installer, or Apple notarization inputs are missing.
It builds the canonical Swift menubar and Go engine into one Pantheon.app,
then signs, notarizes, staples, validates, and publishes both the DMG and the
same-payload PKG before a release record or cask update can be created.

## Stack Lab

- Wing: `stacklab.wing.pantheon-pt`.
- Recipe: `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`.
- The recipe records the paired-DMG-and-PKG credential boundary.
- The tagged release becomes the canonical source starting point for the next
  Pantheon candidate after exact artifact readback.

## Boundaries

This source receipt does not claim a commercial artifact, release record, cask,
or installed-host lifecycle qualification. Those claims require exact workflow
artifact readback and separate host evidence.
