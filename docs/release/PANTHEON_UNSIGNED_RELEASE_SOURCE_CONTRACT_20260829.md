# Pantheon unsigned release-source contract

This contract closes source identity and transition mechanics before any
credentialed release activity. It does not authorize signing, notarization,
asset upload, cask publication, installation, service mutation, or host
qualification.

## Candidate identity

The candidate is addressed by an immutable commit and tree. Its intended
annotated tag must be `v` plus the exact root `VERSION`, and
`scripts/verify-release-source-identity.sh --pretag` must accept that commit
while the tag is absent or already points to that same commit.

For the current lineage, `02029c5f10b117149bbdf02bde4b4367dd632eac` is the
accepted native-default-path parent. The successor receipt, created outside
the source commit after verification, binds the final commit/tree/branch and
the intended `v0.23.14-beta` tag.

## Package and cask transition

Run:

```text
bash scripts/verify-pantheon-unsigned-release-source.sh --pretag v0.23.14-beta <exact-commit>
```

The verifier requires the DMG identity verifier plus the cask renderer and
asset verifier, lifecycle-plan generator, and lifecycle-receipt validator. It
intentionally accepts the checked-in older cask only while
it does not name the unsigned candidate version. A cask update may occur only
after an exact, signed, stapled, uploaded DMG exists and both
`render-pantheon-homebrew-cask.sh` and `verify-pantheon-release-asset.sh`
accept its tag, source commit, tree, asset filename, and SHA-256.

## Residual owner gates

- A visible Developer ID Application identity and complete notarization
  credentials.
- Exact signed and stapled DMG bytes for the tagged commit.
- Clean M1/M5 install, upgrade, rollback, and uninstall receipts. For each
  host, create the plan with `create-pantheon-lifecycle-plan.sh` from the exact
  signed DMG, then validate the independently host-produced receipt with
  `validate-pantheon-lifecycle-receipt.sh`. These scripts are evidence
  contracts only: they do not mount, install, upgrade, roll back, or uninstall
  an app.
- Sustained resource/crash and native accessibility acceptance evidence.

No source verification can substitute for any of those physical or owner-only
proofs.
