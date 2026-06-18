---
id: 20260611-codex-finalwishes-connectrpc-admin-read-sweep
author: codex-finalwishes
addressed_to: claude-home
topic: finalwishes-tier1-ga
repo: /Users/thekryptodragon/Development/FinalWishes
agent_scope: repo-segmented-review
source_items:
  - 20260611-043050-claude-home-codex-finalwishes-scoped-sweep-for-remaining-ungated-connectrpc-admin-sdk-read
status: complete
verdict: READ_PATH_CLEAN_WITH_SEPARATE_WRITE_FINDING
---

# Scoped Sweep: ConnectRPC / Admin-SDK Read Paths

/plan: inspect FinalWishes ConnectRPC service registration, enumerate EstateService methods, trace Firestore/admin-SDK reads in each ConnectRPC method, and report any remaining ungated read path in the `0c2ba2f` blind-spot class.

/goal: close the scoped `codex-finalwishes` item with findings or clean result.

## Verdict

READ-PATH CLEAN for the scoped class.

I did not find another remaining ungated ConnectRPC/admin-SDK read path in the EstateService surface. The four fixed by `0c2ba2f` now have gates, and the other read methods I inspected already gate before acting on `req.Msg.EstateId`.

## Evidence

ConnectRPC is mounted in `/Users/thekryptodragon/Development/FinalWishes/api/cmd/api/main.go` through:

- `estatev1connect.NewEstateServiceHandler(estateService)`
- router group using `authMiddleware`

EstateService methods enumerated in `/Users/thekryptodragon/Development/FinalWishes/api/internal/service/estate/service.go`:

- `GenerateUploadUrl` -> `checkEstateAccess(..., true)`
- `ListBeneficiaries` -> `checkEstateAccess(..., false)`
- `AddBeneficiary` -> `checkEstateAccess(..., true)`
- `ListEstates` -> derives UID from `auth.UserIDFromContext(ctx)`
- `RegisterEstate` -> see separate write-path finding below
- `GetEstateMetadata` -> `checkEstateAccess(..., false)`
- `ListAssets` -> `checkEstateAccess(..., false)`
- `AddAsset` -> `checkEstateAccess(..., true)`
- `ListVaultDocuments` -> `checkEstateAccess(..., false)`
- `ListMemoirs` -> `checkEstateAccess(..., false)`
- `UploadMemoir` -> `checkEstateAccess(..., true)`
- `GetObituary` -> `checkEstateAccess(..., false)`
- `SaveObituary` -> `checkEstateAccess(..., true)`
- `GetAIInsight` -> `checkEstateAccess(..., false)`
- `GetGovernanceSettings` -> `checkEstateAccess(..., false)`
- `ListNotifications` -> `checkEstateAccess(..., false)`

Commands used:

```bash
rg -n "func \\(s \\*Server\\) [A-Z].*\\(ctx context\\.Context, req \\*connect\\.Request" api/internal/service/estate/service.go
rg -n "Collection\\(|Doc\\(|Documents\\(|Get\\(ctx\\)|connectrpc|connect\\.NewRequest" api/internal -g '*.go'
go test ./internal/googlephotos ./internal/opensign ./internal/capsules ./internal/docintell ./internal/transcription ./internal/service/estate
```

Targeted Go tests passed for the reviewed server packages:

- `internal/googlephotos`
- `internal/opensign`
- `internal/capsules`
- `internal/service/estate`
- `internal/docintell` and `internal/transcription` have no test files

## Separate P1 Write-Path Finding

This is outside the narrow "ungated read path" ask, but it is the same trust-boundary family and should be routed to `claude-finalwishes`:

`RegisterEstate` still writes:

```go
"user_id": req.Msg.UserId,
```

The method is authenticated, but it persists caller-supplied ownership identity. Fix by deriving creator UID from `auth.UserIDFromContext(ctx)`, ignoring/removing request `UserId`, and adding a regression test for mismatched request UID.

