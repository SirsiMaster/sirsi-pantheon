# RA-P02 — Known-failure loop (Ma'at registers, Stack Lab remembers)

Owner: Ra, with claude-pantheon for Ma'at. Source: `internal/maat/knownfail/`, `cmd/sirsi/maatknownfail.go`, recognition in `internal/router/wake.go`. Revision: main `c1a78077`.

## Logical view
```mermaid
flowchart TD
  F[Consumer output from a failed or quarantined lane] --> M{Signature matches a catalog entry?}
  M -- yes --> A[Publish the catalogued answer in lane state] --> END1([Lane retries with the answer])
  M -- no --> R[sirsi maat known-failures register: new entry, unresolved]
  R --> FX[Fix ships in a PR with a guard test]
  FX --> RS[sirsi maat known-failures resolve: needs fixed_in and a guard test that exists in the repo]
  RS -- refused --> R
  RS -- accepted --> C[Entry resolved in catalog.json]
  C --> SL[Catalog is a component of ra-horus-fabric-recipe-v1 - Stack Lab pins the origin copy]
  SL --> M
```
Implemented: matching, registration, resolve refusal without `fixed_in` and an existing guard test. Not implemented: an entry that applies its own fix (OPEN).

## Data view
```mermaid
flowchart LR
  OUT[Consumer output] --> WL[wake-loop]
  CAT[(internal/maat/knownfail/catalog.json)] --> WL
  WL -->|answer text| STATE[(Lane state via relay and service)]
  REG[known-failures register / resolve] -->|writes entry| CAT
  CAT -->|component| RECIPE[contracts/stacklab/ra-horus-fabric-recipe-v1.json]
  RECIPE -->|content hash| SLAB[(sirsi-stacklab pin on origin)]
```

## Failure and recovery
- Resolve without a guard test that exists: refused.
- No match: the failure stays a hand repair until registered. This is the gap the loop exists to close.
- Git calls strip the hook's `GIT_*` environment so the guard-test check cannot be fooled by a hook context.
