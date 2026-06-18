---
from: "claude-home"
to: "user"
title: "FW PR #4 follow-up DECISION: signer = caller (current) vs estate principal (recommended). claude-home + claude-finalwishes aligned on B."
type: "decision"
status: closed
opened: 2026-06-11T02:55:19Z
closed: 2026-06-14T18:13:49Z
---

## Instructions

**Decision needed — FinalWishes PR #4 signer semantic question** (claude-home + claude-finalwishes aligned, surfacing).

PR #4 (OpenSign create-side estate-binding) merged tonight. Closes the substitution attack — a writer can no longer name an arbitrary `signerEmail` in the request body; signer is forced to the authenticated caller's email from the verified token claim. **Security is done.**

**Open semantic question**: for a personal legal directive (advance directive, POA), should the signer be:
- **(A) The authenticated caller** (current behavior) — works when caller == principal (normal case), but if an executor with writer role initiates a ceremony for the principal, the executor accidentally gets recorded as the document signer of the principal's directive. That's legally void.
- **(B) The estate principal** (claude-finalwishes recommendation + claude-home endorsement) — server resolves principal email from estate metadata, principal gets the signing link, principal signs their own document. Caller is recorded as `initiated_by` in audit only.

claude-finalwishes is implementing (B) on your confirmation. Both of us recommend (B) — it's the legally correct shape for personal legal directives; current code is secure but semantically wrong for non-principal-initiated ceremonies.

**Reply with `B` (or `A`) and claude-finalwishes will build the follow-up.** If you want extra constraints (e.g., reject if principal email unverified, or allow ceremony to fail open with `initiated_by` not propagated), say so in your reply.

— claude-home (surfacing aligned recommendation, 2026-06-11 02:55 UTC)

## Result

DECIDED: B — the SIGNER = the estate PRINCIPAL (not the caller), for legal directives/POA. Owner deferred to the claude-finalwishes + claude-home recommendation and directed "do it all". claude-finalwishes is GO to build the follow-up: server resolves the principal email from estate metadata; signerEmail/signerName = principal; caller recorded as initiated_by in audit only; reject if principal email unverified. Route the PR to claude-home for review. Reversible — it's a code follow-up, not a live change. — claude-home, owner-directed, 2026-06-14
