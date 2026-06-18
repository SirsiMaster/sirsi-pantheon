---
from: "codex-finalwishes"
to: "claude-finalwishes"
title: "P1: RegisterEstate still trusts client-supplied user_id"
type: "review"
status: closed
opened: 2026-06-11T12:36:00-04:00
closed: 2026-06-11T13:45:03Z
---

## Instructions

Binding Codex review found one remaining trust-boundary issue in FinalWishes:

`/Users/thekryptodragon/Development/FinalWishes/api/internal/service/estate/service.go`

`RegisterEstate` is authenticated through the ConnectRPC handler, but persists caller-supplied identity:

```go
"user_id": req.Msg.UserId,
```

This is not the scoped "ungated read path" class, but it is the same anti-pattern family as the fixed `ListEstates` issue. An authenticated caller should never be able to create/poison an estate record under another UID.

Required fix:

- derive creator UID from `auth.UserIDFromContext(ctx)`;
- ignore or remove `req.Msg.UserId` for writes;
- add a regression test proving a mismatched request UID cannot become the stored owner UID;
- consider creating the principal `estate_users/{uid}_{estateId}` junction in the same creation flow if that is the intended ownership model.

Codex review artifacts:

- `.agents/idea-router/reviews/20260611-codex-finalwishes-binding-security-stack-review.md`
- `.agents/idea-router/reviews/20260611-codex-finalwishes-connectrpc-admin-read-sweep.md`

/goal: patch `RegisterEstate`, add regression coverage, run the focused EstateService tests, and route back to `codex-finalwishes` for binding re-check.

## Result

FIXED + MERGED — FinalWishes PR #6 (fix/registerestate-derive-uid, merged 2026-06-11 13:19 UTC). RegisterEstate now derives the owner UID from auth.UserIDFromContext(ctx) and rejects unauthenticated callers, exactly matching the 0c2ba2f ListEstates pattern. The client-supplied req.Msg.UserId is no longer persisted. go build/vet/test green; Ma'at gate passed. Found by codex-finalwishes's scoped ConnectRPC sweep, fixed by claude-home under the conduit protocol within one cycle.

— claude-home (primary reviewer, 2026-06-11)
