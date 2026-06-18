---
from: "claude-home"
to: "user"
title: "OWNER ACTION: provision SIRSI_SIGN_HMAC_SECRET + API_KEY per tenant (hard prereq for ADR-047 shared-services signing — verified PASS on code)"
type: "decision"
status: closed
opened: 2026-06-11T13:57:06Z
closed: 2026-06-14T18:13:49Z
---

## Instructions

**OWNER ACTION (new) — SIRSI_SIGN secrets are a hard prerequisite for shared-services signing.**

FinalWishes shipped ADR-047 (da2182f) — tenant apps now consume the Sirsi Sign service FIRST, with dissociated self-hosted fallback ONLY on availability failure (transport/5xx). claude-home verified the security-critical logic: it correctly does NOT fall back on a 4xx (so a broken request or auth can't be laundered through the fallback path).

The consequence you need to know: because auth failures (401/403) are 4xx, if the tenant HMAC secret is missing/wrong, signing **fails closed** rather than falling back. That's the secure behavior — but it means provisioning the secret is REQUIRED for the Sirsi-first path to work at all:

- `SIRSI_SIGN_HMAC_SECRET` + `SIRSI_SIGN_API_KEY` → each tenant's Secret Manager (FinalWishes-prod, and assiduous-prod once claude-assiduous mirrors it), bound to Cloud Run.
- Cross-project isolation is correct by design (each tenant gets its own secret in its own project).

Until provisioned, FinalWishes signing returns "Sirsi service failed and no dissociated fallback configured" (or falls back if you've kept OPENSIGN_* configured). Verified PASS on the code; this is the ops half.

— claude-home (primary reviewer, 2026-06-11)

## Result

SUPERSEDED — discovery (claude-finalwishes, claude-home-verified) proved this was based on a wrong assumption: there is NO SIRSI_SIGN shared secret, and the sirsi-opensign project is a defunct out-of-org ghost. The LIVE service is opensignApi in sirsi-nexus-live, whose real issue is the committed-public-default HMAC — now staged + in coordinated cutover (router 20260614-181321). This item is replaced by that work. — claude-home, owner-directed "do it all", 2026-06-14
