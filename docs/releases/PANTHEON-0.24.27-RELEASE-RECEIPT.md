# Sirsi Pantheon v0.24.27 release receipt

Date: 2026-09-28

This patch makes the canonical Swift menubar a mandatory part of Pantheon's
macOS package composition. The Makefile and DMG builder now fail closed when
`macapp/Package.swift` is absent and no longer substitute the retired Go/systray
menubar.

## Exact source and validation

- Release candidate base: `dfee2de41827a9cd90fc407190642b4a493c7604` / tree from the merged PR #850.
- Change source: PR #850, head `25ae5e9435063b3492b87266373e5ffd604e0008`.
- Independent SSA source/static review: ACCEPT; protected PR checks passed.
- Local verification: `git diff --check`, Bash syntax, commercial-release-contract verifier, `go test ./internal/stacklab ./internal/packageinventory ./internal/packageinventorycmd`.

## Stack Lab

- Wing: `stacklab.wing.pantheon-pt`.
- Recipe: `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`.
- The recipe explicitly names the canonical Swift source as an input and the
  canonical Swift menubar as the only native menubar output.
- This release becomes the canonical source starting point for the next
  Pantheon candidate after the tagged workflow publishes its artifacts.

## Boundaries

This source receipt does not by itself claim Developer ID signing, notarization,
stapling, remote artifact publication, Homebrew cask publication, or installed-
host lifecycle qualification. Those claims require the tagged release workflow
and its exact artifact readback evidence.
