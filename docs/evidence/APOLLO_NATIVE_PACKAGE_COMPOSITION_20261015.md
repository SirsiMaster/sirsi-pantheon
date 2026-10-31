# Native installed command package composition

Base: maintenance source36db06b15a543fc02d15b2684a9e0f75fc591952, preserving its router identity/cache/worker repairs. Client and admission source: exact nine Apollo-specific files from4af9abff8264947ca146abfcb6edfbe007c2389a. Apollo SDK:9d24c8c9262c81c2edac811e96e37f9b598074a1. A broad merge was aborted before changes were authored; older unrelated Gemma/router source was not substituted for current maintenance code.

Package addition: explicit PANTHEON_APOLLO_PROFILE installs the private profile into Contents/Resources/Apollo/native-profile.json BEFORE signing. Both app and expanded PKG inventory require the supplied exact canonical bytes. Generic packages still reject an undeclared Apollo resource; the allowlist admits no other files under that directory. Profiles and host paths are not committed into this repository.

## Closed release capability contract

The current package contract records `SirsiApolloNativeCapability=enabled` or
`disabled` in the signed app Info.plist. Release builders require an explicit
`PANTHEON_APOLLO_CAPABILITY` selection. Native-enabled builds require an
absolute regular non-symlink profile, bind its exact bytes through canonical Go
inventory before app signing, after app signing, and after PKG expansion, and
refuse to sign if the profile is missing or unsafe. Native-disabled builds
forbid the profile. The independent inventory expectation must match the signed
plist value and profile presence; the plist is not accepted as its own
authority.

Published v147 contains no native Apollo profile, so the normal release
workflow explicitly selects `disabled`. Development builds remain optional:
with no capability variable they infer enabled only when the existing profile
input is supplied, and otherwise record disabled. Historical developer
inventory inputs without this key remain readable only through the
`legacy-development` verifier mode. At runtime, missing or malformed capability
metadata is unavailable, never interpreted as disabled. The app-embedded CLI
reads its own bundle; the managed PKG CLI reads the canonical app resources
only after its bytes match the app-embedded CLI. The Qwen Apollo 226 HTTP route
continues before this native-only capability check.

These source checks do not certify a signed or installed release. SSA retains
signing/publication authority; runtime profile and model qualification remain
separate evidence.

The existing native runtime and model shards are in durable per-user private directories. They retain the actual source/code/model hashes proven by Apollo's direct Thunderbolt run. The profile is not evidence of numerical qualification. The prior64-token31B Q8 construction measured9.428tok/s under loaded conditions; this is not a12B Plain comparison or a commercial distributed-release qualification.

Focused Apollo/authority compatibility, package inventory, canonical file adapter, standalone inventory, and Stack Lab tests pass. New negative controls reject an unbound profile, changed bytes and a missing profile. Exact build/signature/installation and normal-command evidence must be appended after execution, never inferred from these tests.

SSA confirms final native-capable candidate0.24.104; earlier maintenance104 build artifacts are unpublished precursors, not a different release. The intermediate source-only105 label was reverted before any build/sign/install. SSA owns all final signing/install/live activation. PKG now copies the signed app CLI to the existing /usr/local/libexec/sirsi-pantheon/sirsi payload path and compares bytes both before pkgbuild and after archive expansion. No new daemon, plist, credentials or restart is introduced by assembly.
