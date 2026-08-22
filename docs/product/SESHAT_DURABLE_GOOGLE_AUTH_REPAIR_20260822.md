# Seshat Durable Google Authentication Repair

Date: 2026-08-22  
Status: source and focused-test repair accepted; one-time Sirsi desktop OAuth client provisioning remains operationally open

## Root cause

Seshat treated a short-lived Google access token as a permanent credential. It
never used a refresh token, never persisted a renewed token, and translated a
Drive HTTP 401 into instructions to authenticate again. Its CLI printed Google's
retired out-of-band authorization URL rather than completing a supported desktop
OAuth flow.

This was separate from both Firebase and gcloud:

- Firebase CLI is durably logged in as `admin@sirsi.ai`, has a refresh token,
  and completed a non-interactive four-project API request.
- gcloud ADC exists but a direct Drive request returned HTTP 403 for insufficient
  scopes.
- Seshat had neither `google_credentials.json` nor `google_token.json`.
- The only downloaded OAuth client belongs to `finalwishes-prod`, is a web client
  limited to FinalWishes origins, and was correctly rejected across the Sirsi
  property boundary.

## Repair

- Replaced the retired copy/paste flow with a loopback desktop OAuth callback.
- Added PKCE, cryptographically random state, a three-minute bounded callback,
  Drive-read-only scope, and exact state/error validation.
- Added proactive access-token renewal and one bounded refresh/retry after 401.
- Preserved refresh tokens when Google rotates only the access token.
- Added atomic, fsynced, owner-only (`0600`) token persistence.
- Added status inspection that reports only configured/authorized/refreshable
  state and never token values.
- Missing or revoked consent fails with one actionable instruction rather than
  reopening authentication indefinitely.

## Evidence

- `go test ./internal/seshat ./cmd/sirsi`: pass.
- Refresh/persistence regression proves renewed access, retained refresh token,
  and `0600` permissions.
- Fixed CLI SHA-256:
  `3ce510a7d89c0f928be565009a1f9616458eec6b0622af2d347826393a8eb4c4`.
- Firebase non-interactive project listing: success, four projects.
- Existing gcloud ADC Drive check: HTTP 403, insufficient scopes.

## Remaining one-time operation

Create or provide a Sirsi-owned Google OAuth desktop client with Drive API access
at `~/.config/seshat/google_credentials.json`, then run
`sirsi seshat auth google` once. Seshat will thereafter renew silently unless the
owner revokes consent. No FinalWishes credential may be reused.

