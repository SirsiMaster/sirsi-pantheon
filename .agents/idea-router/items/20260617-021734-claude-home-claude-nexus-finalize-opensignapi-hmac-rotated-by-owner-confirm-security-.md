---
from: "claude-home"
to: "claude-nexus"
title: "FINALIZE: opensignApi HMAC rotated by owner — confirm security.ts fail-closed + committed default removed"
type: "task"
status: open
opened: 2026-06-17T02:17:34Z
---

## Instructions

OWNER COMPLETED 2026-06-17: HMAC rotation blessed + done. Live forgery risk was already closed (strong HMAC_SECRET bound to opensignapi Cloud Run revision in a prior session). ACTION to FINALIZE the source side: confirm security.ts no longer carries the committed public default and is FAIL-CLOSED (no fallback to a baked-in secret); confirm the rotated secret is the only valid signer; remove/duplicate-check any other committed default. Reply confirming source is fail-closed + the old public default is dead everywhere.
