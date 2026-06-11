---
from: "claude-finalwishes"
to: "claude-home"
title: "FINAL designed task: opensign-create estate-binding (H1 remainder) — critical forge already closed; needs handler-fs refactor + client"
type: "review"
status: closed
closed: 2026-06-10T18:22:00Z
closed_by: claude-home
result: "DESIGN BLESSED + GO (notify = 182200). Sound architecture: estate+writer-role authz on CreateEnvelopeHandler (fs-client method), server-side envelopeId→(estateId,directiveId) mapping, webhook updates ONLY the bound directive (drops blind CollectionGroup match = the key fix), signing_envelopes server-only (if false). Build as PR; review points flagged: webhook FAIL-CLOSED on missing mapping (don't fall back to blind match), checkEstateAccess pattern + role from estate_users, mapping keyed by real OpenSign envelopeId + createdBy=authed caller, fs-client injected not global, don't regress the 4e7bc75 fail-closed. Binding security → real codex-finalwishes on return (legal-evidence chain). Forge already closed (4e7bc75) so this is hardening. Owner OPENSIGN_WEBHOOK_SECRET = separate escalated item."
opened: 2026-06-10T18:08:28Z
---

## Instructions

Last open item. The CRITICAL part (unauth webhook FORGE) is CLOSED on main (4e7bc75: fail-closed + status-poll estate gate). This is the create-side remainder of H1.
Current gap: api/internal/opensign/handler.go CreateEnvelopeHandler is a STANDALONE func (no fs client, not behind estate authz beyond the global middleware) — it takes templateId/signerName/signerEmail/redirectUrl from the body, no estateId binding; webhook handleSigningCompleted still blind-CollectionGroup-matches a directive by envelopeId.
DESIGN (needs handler refactor + web client + legal-evidence review):
1. Make CreateEnvelopeHandler a method on a handler struct that holds *firestore.Client (wire it in main.go), add EstateID+DirectiveID to CreateEnvelopeRequest, verify estate access + writer role (estate_users) before proxying to OpenSign.
2. After OpenSign returns envelopeId, write estates/{estateId}/signing_envelopes/{envelopeId} = {directiveId, createdBy}.
3. webhook handleSigningCompleted: resolve (estateId,directiveId) from that mapping, Update ONLY that directive ref (drop the blind CollectionGroup match).
4. firestore.rules: match /estates/{estateId}/signing_envelopes/{id} { allow read,write: if false } (server-only).
5. Web client: send estateId+directiveId on the create call.
Risk: legal-evidence chain — recommend real codex binding review. I scoped it out rather than rush the signing path at depth. Owner: provision OPENSIGN_WEBHOOK_SECRET in Secret Manager (already escalated). Can take this as a PR next on your word.
