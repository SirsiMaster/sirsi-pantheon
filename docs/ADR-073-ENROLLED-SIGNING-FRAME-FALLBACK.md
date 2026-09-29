# ADR-073 — Enrolled Signing-Frame Fallback

**Status:** Proposed
**Date:** 2026-09-29

## Context

The commercial macOS release job failed because the optional CI certificate
bundle arrived empty even though the self-hosted release machines were already
enrolled with the owner-approved Developer ID identities. Treating the bundle
as mandatory made the release path unable to use its authorized local signing
frame.

## Decision

The release workflow has two mutually exclusive signing paths:

1. When `MACOS_CERTIFICATE` is present, retain the existing temporary-keychain
   import path and require its password inputs.
2. When it is absent, select the self-hosted runner's login keychain without
   exporting, importing, copying, or otherwise handling private key material.

Both paths resolve and require the exact Team `9D382WV988` Developer ID
Application and Developer ID Installer identities before building. The
workflow fails closed if the selected keychain or either identity is absent or
does not match. Notarization continues to use the existing owner-managed Apple
credentials; this ADR does not claim a signed or notarized release by itself.

## Verification and rollback

The tagged release workflow must prove the selected keychain mode, exact
identities, signed DMG/PKG, notarization, staple/readback, and canonical cask
bytes. If any proof fails, the tag remains noncommercial and no release asset
is promoted. Revert this workflow and version change to restore the prior
mandatory-bundle behavior.
