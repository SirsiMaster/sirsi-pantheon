### Fixed — preserve optional Apollo profile argv under macOS Bash 3.2

- Initialize the nonempty Apollo inventory argument array with the capability
  pair in both DMG and PKG builders, then append the profile pair only when a
  profile is present. Remove the empty optional array that caused Bash 3.2
  nounset to abort disabled-profile DMG builds.
- Deliberately refresh Ma’at release-preflight provenance hashes for both
  builder files; this keeps their exact reviewed Bash 3.2 argv correction
  pinned without changing any signing, keychain, or contract checks.
- Add a regression to the existing macOS artifact-contract suite. It extracts
  the production argument blocks and first inventory commands from each
  builder, runs them under `/bin/bash` 3.2 against an owned recording CLI, and
  checks exact disabled/enabled argv. The enabled DMG fixture also checks only
  its temporary copied profile bytes and mode. No build, package, signature,
  mount, notarization, or model operation is performed by the fixture.

Refs: `contracts/stacklab/pantheon-release-artifact-recipe-v1.json`,
`scripts/PACKAGING.md`, `.github/workflows/ci.yml`.
