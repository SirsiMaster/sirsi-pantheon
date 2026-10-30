fix(core): bind Apollo capability in package inventory

Bind the signed Apollo capability to an independent release expectation and
exact protected profile bytes across app signing and expanded PKG verification.
The published-v147 release workflow declares native Apollo disabled; developer
builds retain optional profile input and legacy inventory compatibility.
Runtime resolution accepts only the app-embedded CLI or a byte-matched managed
CLI, and refuses missing or malformed capability metadata as unavailable.

These source changes and fixtures do not certify a signed or installed release;
signing, publication, and installed-runtime evidence remain separate.

Refs: docs/evidence/APOLLO_NATIVE_PACKAGE_COMPOSITION_20261015.md, contracts/stacklab/pantheon-release-artifact-recipe-v1.json
