---
from: "claude-home"
to: "user"
title: "CRITICAL: SirsiNexus opensignApi HMAC = committed public default (forgeable legal-doc certificates) — bless the rotation; FW prepping fix"
type: "decision"
status: closed
opened: 2026-06-14T17:50:25Z
closed: 2026-06-17T02:17:34Z
---

## Instructions

**CRITICAL (owner action) — SirsiNexus opensignApi HMAC secret is the committed public default.**

claude-finalwishes found + claude-home source-confirmed a CRITICAL on the SHARED prod signing service (sirsi-nexus-live `opensignApi`):

`packages/sirsi-auth/functions/src/security.ts:4` ships
`HMAC_SECRET = process.env.HMAC_SECRET || 'sirsi-opensign-hmac-secret-key-2025-v1-CHANGE-IN-PRODUCTION'`
— a committed public default, in a PUBLIC repo. The live opensignApi has NO HMAC_SECRET env, so the public default is ACTIVE. That HMAC gates: (1) request auth (opensign.ts:94), (2) signed tokens (:106), and (3) the "Security Hash / CERTIFICATE OF COMPLETION" on EXECUTED legal MSAs (:119, the $200K agreement). Anyone with repo read access can forge request signatures AND fabricate a legally-"executed" agreement that validates.

**Why it needs YOU:** it's a shared prod service SirsiNexus itself consumes; rotating the secret can break in-flight signing flows. claude-finalwishes is authorized to PREP the fix (generate a strong secret, draft the Secret Manager + Cloud Run bind + redeploy, patch security.ts to fail-closed and remove the committed default) and route it to you as a PR. You bless the actual rotation + decide whether outstanding signed artifacts (issued under the public secret) need re-issuance.

**Also gates ADR-047:** FinalWishes can't safely consume Sirsi Sign until this secret is fixed (consuming a service whose shared secret is public = consuming broken auth). FW stays on its secure dissociated fallback meanwhile — no regression. ADR-047 Item 1 is correctly BLOCKED on this.

Full verdict + threat model: router item 20260614-172508 (closed with my analysis). Prepped fix incoming from claude-finalwishes for your approval.

— claude-home (definitive reviewer, 2026-06-14)

## Result

CLOSED — owner reports COMPLETED 2026-06-17: HMAC rotation done (owner blessed + rotated). Live forgery risk already closed prior (strong secret bound to opensignapi); source-level cleanup now to finalize.
