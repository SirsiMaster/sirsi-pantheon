# Pantheon SNE Canonical Engine and Live Authorization Evidence

**Date:** 2026-08-21  
**Status:** accepted pilot boundary; signed-native Keychain boundary pending  
**Scope:** Pantheon DMG, standalone menubar, CLI dashboard, GUI dashboard, Nexus local inference handoff

## Incident

Pantheon's release channels could silently ship different executables under the
same `sirsi-menubar` name. The DMG script selected the SwiftUI prototype whenever
`macapp/Package.swift` existed. The standalone release archive always built
`cmd/sirsi-menubar`. The SwiftUI prototype is a presentation surface and does not
start the protected loopback API, SNE lifecycle controller, recovery controller,
durable local capability, or Nexus capability handoff. Therefore the DMG could
omit operational features that were present in the standalone archive.

Two additional constructors, `sirsi dashboard` and `sirsi-gui`, also started the
dashboard without provisioning `SNELocalAccessToken`. They could therefore enter
the migration-only empty-token mode even though the menubar enforced bearer
authorization.

## Repair

1. `scripts/build-dmg.sh` now always builds `./cmd/sirsi-menubar/`, matching the
   standalone release workflow.
2. `scripts/verify-menubar-release-contract.sh` rejects conditional Swift binary
   substitution and proves both release channels name the canonical Go entrypoint.
3. `.github/workflows/release.yml` runs that verifier before building the DMG.
4. `sirsi dashboard` and `sirsi-gui` now load the same private, restart-stable
   local capability and its durable path. They refuse startup if the private
   capability store cannot be safely established.
5. The SwiftUI implementation remains a valid future native presentation surface.
   It may become the app executable only after it supervises the same packaged Go
   engine and passes this release contract. It must not reimplement engine truth.

## Executed evidence

- Shell syntax for the DMG and verifier scripts: accepted.
- Release contract: `accepted=true canonical_entrypoint=cmd/sirsi-menubar channels=dmg,standalone`.
- Canonical arm64 menubar build: accepted; linker emitted only the existing
  duplicate `-lobjc` warning.
- Focused Go suites: `cmd/sirsi`, `cmd/sirsi-gui`, `cmd/sirsi-menubar`, and
  `internal/dashboard` all passed.
- Live isolated server: `http://127.0.0.1:19119`.
- Missing bearer: HTTP 401 `local_capability_required`.
- Invalid bearer: HTTP 403 `local_capability_invalid`.
- Correct durable bearer: crossed authorization and returned HTTP 503
  `sne_not_ready`, the expected readiness result when no verified runtime is
  active.
- All responses declared `no_fallback=true`.
- The isolated server was terminated cleanly after capture.

### Embedded dashboard continuity

The first authorization repair exposed a usability regression: Pantheon's
embedded dashboard issued credential-free `fetch()` calls, so its own protected
SNE lifecycle and recovery controls would receive 401. The repair establishes a
session-only `sirsi_sne_local_session` cookie when serving the strict-loopback
overview page. It is `HttpOnly`, `SameSite=Strict`, and scoped to `/`; page
JavaScript never receives the root value. The capability middleware accepts this
cookie only when no Authorization header is supplied and only after the existing
strict Host/Origin gate. An explicit invalid bearer can never fall back to the
cookie.

Live evidence on isolated port 19120:

- overview response set `HttpOnly; SameSite=Strict; Path=/`;
- cookie-authenticated `POST /api/sne/stop` returned HTTP 200;
- the same cookie with `Origin: https://attacker.example` returned HTTP 403
  `origin_not_allowed` and `no_fallback=true`;
- the isolated server shut down cleanly.

## Claim boundary

This proves one canonical engine entrypoint across the current DMG and standalone
release scripts and proves live local bearer enforcement for the headless engine.
It does not yet prove a signed/notarized installed DMG, native Nexus Keychain
handoff, or a ready model response. This host reported zero valid code-signing
identities during the run. The current mode-0600 capability file is acceptable for
the pilot boundary but not the final same-user-process threat boundary. GA requires
a Developer-ID-signed Pantheon/Nexus pair using a Data Protection Keychain access
group and a native/XPC handoff that does not expose the root capability to hosted
JavaScript.

## Permanent prevention rule

No release channel may select an executable by source-tree presence. Release
identity must be explicit and verified. Every constructor of the Pantheon local
API must supply the same non-empty authorization boundary or refuse startup.

## Signing-key audit

Two valid Apple Developer ID Application certificates are present on the M5:

- serial `644D541EAC0B3E92091F1728016C3E7E`, valid through 2031-06-20;
- serial `28EA4DAC7C25F4F7966E70E9D16BC854`, valid through 2031-08-17.

The newer certificate was already installed in the login keychain. Direct
private-key searches found no matching key in either the login keychain or
`Sirsi-SNE-Release-v4.keychain-db`. The authorized M1 also reported zero valid
code-signing identities. Therefore neither machine can currently form a signing
identity from these certificates. GA signing requires a new local private key and
CSR, followed by an Apple-issued certificate for that key. Never treat a `.cer`
file by itself as a signing identity.

A replacement signing key and CSR were then created with Apple's native
`certtool`, so the private key was born inside the login Keychain and was never
written as a loose file. Evidence:

- Keychain label: `Sirsi Technologies Developer ID 2026-08-21`;
- RSA key size: 2048 bits;
- CSR signature: verified successfully by both `certtool V` and OpenSSL;
- CSR SHA-256: `a980e404f33da5b85e97de18571917c956cdbdbff30d43bbd4ac48d8cfb34373`;
- CSR path: `~/Library/Application Support/Sirsi/Signing/Sirsi-Technologies-Developer-ID-20260821.csr`.

Apple issuance remains pending because the Developer portal session requires a
fresh owner sign-in. The CSR has not been uploaded. After sign-in, request a
Developer ID Application certificate with this exact CSR, import the returned
certificate, and require `security find-identity -p codesigning` plus an actual
signed-binary verification before enabling shared-Keychain entitlements.
