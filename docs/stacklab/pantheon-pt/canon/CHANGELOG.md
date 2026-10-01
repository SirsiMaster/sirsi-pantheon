# Changelog — Pantheon PT canon

## 2026-10-01 — v0.24.65 release candidate

- Records the PR947/PR949/PR950 lineage from the exact tested mainline: lease
  ownership across session remint and threadless invocation, and a fail-closed
  PostgreSQL CI leg.

## 2026-10-01 — v0.24.64 release candidate

- Records the PR944 router-completion release lineage from the exact tested
  mainline, including token-fenced completion, authenticated dual-backend
  coverage, truthful lane-state observability, and reversible router closure.
- The tag and published artifacts become the canonical starting point for the
  next Pantheon build; signing, notarization, cask, and installed-host receipts
  are recorded only after their corresponding release jobs complete.

## 2026-09-28 — v0.24.23 release candidate

- Records the exact tested mainline Stack Lab doctor roster-provenance change
  from PR #843 as the next commercial patch release candidate.
- Release scope remains CLI/Go and canon provenance; macOS signing,
  notarization, Homebrew cask publication, and installed-host qualification
  remain release-workflow evidence rather than source-only claims.

## 2026-09-27

- Added the complete Stack Lab nine-record canon bundle.
- Separated source/static claims from build, package, credential, signing, notarization, remote, cask, and installed-host proof.

## 2026-09-28

- Added the independently cataloged development-versus-commercial release artifact recipe. Commercial artifact names now require the Developer ID/notary/stapling route; ad-hoc package work is explicitly `-dev` only.
