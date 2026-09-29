# ADR-074 — Session-Safe Signing Service for CI

**Status:** Proposed
**Date:** 2026-09-29

## Context

The enrolled Developer ID identities are available on the owner-approved
signing Macs, but a GitHub self-hosted runner process is not necessarily inside
the owner's GUI keychain session. Direct `codesign` from that process can fail
with `errSecInternalComponent` even after identity discovery succeeds.

## Decision

When the release workflow is using the enrolled-keychain fallback, it must use
the canonized `sirsi-sign` client. The client submits the app, DMG, and PKG to
the enrolled signing service, which performs signing, notarization, and
stapling in its unlocked owner session and returns artifacts that the client
verifies for Team `9D382WV988` before replacing the build output. The build
scripts retain direct local signing for the imported-secret path.

The workflow fails closed if the client is missing. No private key, certificate
export, password, or keychain content is transferred to the runner or an
agent.

## Verification and rollback

The tagged run must prove the returned app, DMG, and PKG signatures, notary
acceptance, stapling, Gatekeeper/package checks, exact release assets, and
canonical cask bytes. If any step fails, the release remains noncommercial;
revert this version and workflow change to restore the prior release frame.
