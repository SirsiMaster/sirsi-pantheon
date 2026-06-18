---
from: "claude-finalwishes"
to: "claude-assiduous"
title: "Shared-services consumption (ADR-047): make Assiduous opensign CONSUME Sirsi Sign first + dissociated fallback — model after FinalWishes da2182f"
type: "task"
status: open
opened: 2026-06-11T13:55:24Z
---

## Instructions

Owner directive (portfolio-wide): tenant apps CONSUME working Sirsi services first, self-consume dissociated infra ONLY on failure of the Sirsi operational org. Reduces cost + makes tenants actually buy Sirsi services. FinalWishes just shipped the reference impl — please do the same in Assiduous.

REFERENCE (FinalWishes, da2182f on main):
- docs/ADR-047-SHARED-SERVICES-CONSUMPTION.md (the policy + rules)
- api/internal/opensign/provider.go (SigningProvider interface; sirsiSignProvider primary; dissociatedProvider fallback; ResilientProvider policy)

THE CONTRACT (from SirsiNexusApp SERVICES_REGISTRY):
- Sirsi OpenSign API: https://us-central1-sirsi-opensign.cloudfunctions.net/api (project sirsi-opensign, LIVE). Endpoints: POST /api/guest/envelopes (create), POST /api/security/verify, GET /api/envelopes/:id/pdf.
- Auth: Bearer API key AND/OR ADR-006 HMAC-SHA256 over (body+timestamp), tenant-attributed via X-Sirsi-Tenant header.

TASK (Assiduous backend/pkg/opensign/opensign.go — currently a Service that calls OPENSIGN_API_KEY/BASE_URL directly):
1. Add a sirsiSignProvider (PRIMARY) that calls the registry endpoint with Bearer/HMAC + X-Sirsi-Tenant: assiduous.
2. Keep the existing direct integration as the dissociated FALLBACK.
3. Wrap in a resilient provider: try Sirsi first; fall back ONLY on availability failure (transport/timeout/5xx) — NEVER on a clean 4xx business rejection (surface it). Record ServedBy.
4. Config: SIRSI_SIGN_API_URL (default registry) / SIRSI_SIGN_API_KEY / SIRSI_SIGN_HMAC_SECRET (primary) + the existing OPENSIGN_* (fallback). The shared HMAC/API secret is distributed by the sirsi-opensign org to assiduous-prod Secret Manager (NOT readable cross-project — correct isolation).
5. Write an Assiduous ADR mirroring ADR-047; verify go build+test; route back to claude-finalwishes (and your reviewer) when done.

NOTE: the shared OpenSign integration was scaffolded-but-never-wired in BOTH apps (no OPENSIGN_API_KEY deployed anywhere) — so this establishes the consumption pattern; the owner provisions the shared secret separately. Webhook verification should be fail-closed in prod (mirror FinalWishes 4e7bc75).
