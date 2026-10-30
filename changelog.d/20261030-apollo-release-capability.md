fix(core): bind Apollo capability in package inventory

Bind Apollo capability to an independent release expectation and the exact
protected profile bytes across app signing and expanded-PKG verification. The
release path freezes the initial Info.plist/profile digest pair before signing;
the DMG and paired PKG share a fresh private per-build receipt, and PKG refuses
a missing receipt instead of recapturing an already-built candidate. Receipt
creation is exclusive, checks full writes and close errors, and retains
incomplete receipts. Profile reads require root/current-user ownership and no
group/other write bits. Development keeps optional profiles, legacy capability
behavior, and signed-inventory structure checks without the release-only frozen
digest requirement. Runtime resolution accepts only the app-embedded CLI or a
byte-matched managed CLI and refuses missing or malformed capability metadata
as unavailable. The published-v147 release workflow remains native-disabled.

These source changes and fixtures do not certify a signed or installed release;
signing, publication, and installed-runtime evidence remain separate.

Refs: docs/evidence/APOLLO_NATIVE_PACKAGE_COMPOSITION_20261015.md;
internal/packageinventorycmd/verify.go;
internal/packageinventorycmd/verify_test.go;
scripts/build-dmg.sh; scripts/build-pkg.sh;
scripts/new-apollo-digest-receipt.sh; Makefile;
.github/workflows/release.yml;
contracts/stacklab/pantheon-release-artifact-recipe-v1.json;
scripts/PACKAGING.md; PANTHEON_RULES.md A7/A11/A12/A16; ADR-032.
