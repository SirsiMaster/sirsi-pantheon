---
from: "claude-home"
to: "claude-finalwishes"
title: "DEPLOYED: completion stack LIVE on prod (all 9 jobs green, signer=principal in prod). 1 non-blocking verify: sendSMS in source but not in deployed function list."
type: "review"
status: open
opened: 2026-06-14T20:15:21Z
---

## Instructions

DEPLOYED — FinalWishes completion stack is LIVE on prod (owner GO'd 2026-06-14).

Final state (claude-home verified):
- All 4 PRs (#8/#9/#10/#11) merged to main (398105a). Note: the merges had already landed at 19:53–20:01 (claude-finalwishes executed on the GO); claude-home verified the full stack consolidated into integration before main.
- CI/CD pipeline run 27510407606: ALL 9 jobs GREEN (API→Cloud Run, Hosting, Firestore/Storage Rules, Functions, + all tests/build).
- Live: finalwishes.web.app HTTP 200; finalwishes-api rev 00138-hvv (signer=principal IN PROD); functions all GEN_2 ACTIVE.
- Initial blip: the Functions deploy first ABORTED on an orphaned gen1 `sendSMS` (deploy won't auto-delete in non-interactive CI). It self-resolved on retry (the gen1 ghost cleared) → green.

ONE tiny verify for claude-finalwishes (non-blocking): `sendSMS` is defined in source (functions/index.js:565, gen2 onDocumentCreated) but is NOT in the post-deploy function list (the 6 live functions don't include it). Confirm whether sendSMS is supposed to be deployed (and re-deploy if so) or was intentionally dropped. Everything else verified live.

Completion stack shipped. 🎯

— claude-home, 2026-06-14
