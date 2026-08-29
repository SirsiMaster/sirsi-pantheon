# Pantheon v0.23.14 Release Engineering Closure

**Status:** source-complete up to owner-only signing/notarization credentials;
not a release claim.

## Implemented fail-closed mechanics

- `scripts/build-dmg.sh` derives its default version from root `VERSION` and
  rejects any requested package version that does not exactly match source.
- `scripts/install.sh` no longer falls back to `go install` when no release is
  present. On macOS it refuses archive installation before creating install
  state, preserving the signed native app and its embedded CLI as the only
  product-install surface.
- `scripts/render-pantheon-homebrew-cask.sh` refuses to render a cask unless a
  release JSON proves the exact tag, source commit, source tree, and nonempty
  DMG asset.
- `scripts/verify-pantheon-release-asset.sh` binds tag, source commit/tree,
  local package hash, release asset, and generated cask version/hash/URL.
- `.github/workflows/release.yml` now has one cask mutation path. It downloads
  the uploaded asset, reads back release metadata, renders a cask only after
  that proof, and validates it before updating the tap.
- `scripts/create-pantheon-lifecycle-plan.sh` emits a no-mutation plan bound to
  package SHA and an M1 or M5 profile. `scripts/validate-pantheon-lifecycle-receipt.sh`
  accepts only a host receipt that proves install, upgrade, rollback, uninstall,
  peak RSS, swap growth, and zero leaked process state against that plan.

## Static verification performed

```text
bash -n scripts/build-dmg.sh scripts/install.sh \
  scripts/verify-pantheon-release-asset.sh \
  scripts/render-pantheon-homebrew-cask.sh \
  scripts/create-pantheon-lifecycle-plan.sh \
  scripts/validate-pantheon-lifecycle-receipt.sh
exit 0

ruby -e "require 'yaml'; YAML.load_file('.github/workflows/release.yml')"
exit 0 — release_yaml=valid

git diff --check
exit 0
```

## Residual gates

The only credentialed execution gate is visible Developer ID Application
certificate/private-key availability plus complete App Store Connect
notarization credentials. After that, the remaining physical work is bounded:
build exact v0.23.14 tagged bytes, sign/notarize/staple, read back the release
asset, run the cask binding check, and execute the separately admitted M1/M5
lifecycle plans. No signed artifact, cask update, installed app, service, or
runtime state was changed by this source closure.
