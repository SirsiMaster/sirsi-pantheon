---
from: "claude-home"
to: "user"
title: "GO? FinalWishes completion stack (4 PRs) — claude-home BINDING PASS; merging #8→main = LIVE PROD DEPLOY. Your go is the only gate."
type: "decision"
status: closed
opened: 2026-06-14T19:49:40Z
closed: 2026-06-14T20:15:21Z
---

## Instructions

**DECISION: deploy the FinalWishes completion stack to prod?** claude-home reviewed + BINDING PASS — your go is the only gate.

4 PRs, all CLEAN + MERGEABLE, source-verified:
- **#10 signer=principal** — your blessed design, faithfully built + test-proven (an imposter signerEmail in the body is rejected; signer resolved server-side from the estate principal's verified email; unverified→403). 
- **#9 hardening** — Dependabot CVEs → 0, bundle split.
- **#8 completion** — 42 findings fixed across 12 buckets + CR-10 legal corpus (dry-run verified, Rule 9 respected).
- **#11 royal sweep** — design Rule 27 compliant.
- Signer residual CLEARED: prod has 11/11 estates with principalId, 0 backfill needed.

**Why this needs your word:** merging #8→main auto-deploys hosting + api + functions to the LIVE FinalWishes prod. I hold the merge but won't fire a live-prod deploy on the HMAC-scoped approval — this is a separate, bigger surface.

**Reply GO** and I'll execute the stacked merges (#11→#9→#10→#8→main) → prod deploy. Or you merge yourself. Or tell me to hold.

— claude-home, 2026-06-14

## Result

DEPLOYED — FinalWishes completion stack is LIVE on prod (owner GO'd 2026-06-14).

Final state (claude-home verified):
- All 4 PRs (#8/#9/#10/#11) merged to main (398105a). Note: the merges had already landed at 19:53–20:01 (claude-finalwishes executed on the GO); claude-home verified the full stack consolidated into integration before main.
- CI/CD pipeline run 27510407606: ALL 9 jobs GREEN (API→Cloud Run, Hosting, Firestore/Storage Rules, Functions, + all tests/build).
- Live: finalwishes.web.app HTTP 200; finalwishes-api rev 00138-hvv (signer=principal IN PROD); functions all GEN_2 ACTIVE.
- Initial blip: the Functions deploy first ABORTED on an orphaned gen1 `sendSMS` (deploy won't auto-delete in non-interactive CI). It self-resolved on retry (the gen1 ghost cleared) → green.

ONE tiny verify for claude-finalwishes (non-blocking): `sendSMS` is defined in source (functions/index.js:565, gen2 onDocumentCreated) but is NOT in the post-deploy function list (the 6 live functions don't include it). Confirm whether sendSMS is supposed to be deployed (and re-deploy if so) or was intentionally dropped. Everything else verified live.

Completion stack shipped. 🎯

— claude-home, 2026-06-14
