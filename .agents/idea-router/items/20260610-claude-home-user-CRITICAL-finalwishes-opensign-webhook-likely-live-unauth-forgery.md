---
from: "claude-home"
to: "user"
title: "CRITICAL (likely LIVE in prod): FinalWishes OpenSign webhook is probably unauthenticated → forge signed legal directives. Verify + fix-deploy ASAP."
type: "decision"
status: closed
opened: 2026-06-10T17:50:00Z
closed: 2026-06-14T18:13:49Z
---

## Instructions

claude-finalwishes's round-6 security audit found a CRITICAL that is **likely live in
FinalWishes production right now** — surfacing immediately because it's a legal-evidence
integrity hole on a live estate product.

## The vulnerability
`POST /api/v1/opensign/webhook` **fails OPEN**: when `OPENSIGN_WEBHOOK_SECRET` is unset,
it SKIPS signature verification (in all envs — no prod fail-closed, unlike the Stripe
webhook). The secret appears only in `.env.example` + code — **NOT wired into cloudbuild
/ Cloud Run** — so it's almost certainly unset in prod → the webhook is
**live-unauthenticated**.

Impact: an unauthenticated attacker can POST `{event_type: completed, envelope_id: <guessed>}`
and the server will write `signingVerified: true` + `signedAt` + an attacker-supplied
`signedDocumentUrl` — **forging a server-verified signed legal directive** (advance
healthcare directive / power of attorney). On an estate-planning product, that's the
worst class of breach.

(Also MEDIUM: a cross-estate IDOR on `GET /opensign/status?envelopeId=` lets any logged-in
user read another estate's signing state. Lower priority than the CRITICAL.)

## What needs to happen (your call on urgency)
1. **VERIFY prod exposure** — is `OPENSIGN_WEBHOOK_SECRET` actually set in the deployed
   Cloud Run service? (If unset, the hole is live.)
2. **Fix: fail CLOSED in prod** (mirror the Stripe webhook) + **wire the secret into
   cloudbuild/Cloud Run** + host-allowlist `signedDocumentUrl` + idempotency on the write.
3. **Consider gating the endpoint** (or requiring the secret) until fixed, if it's
   confirmed live-unauthenticated.

claude-finalwishes owns the fix (its repo, standing-auth) — I've blessed the fix
direction (175000) and it can deploy on its verification to close the live hole ASAP.
Binding security review goes to codex-finalwishes on its return (security defense-in-depth).
This item is just to make sure YOU know a likely-live critical is being worked, in case
you want to prioritize or take the endpoint down now.

Ref: claude-finalwishes round-6 audit 174659, api/internal/opensign/webhook.go, my
concurrence 175000.

## Result

SUPERSEDED — the FinalWishes OpenSign webhook fail-OPEN forgery was code-fixed in 4e7bc75 (fail-closed: unset secret in prod → 503 reject, verified by claude-home). The only remaining lever (the webhook secret) folds into the broader OpenSign HMAC story now being executed (the sirsi-nexus-live opensignApi HMAC cutover, secret staged + routed). No separate action. — claude-home, owner-directed "do it all", 2026-06-14
