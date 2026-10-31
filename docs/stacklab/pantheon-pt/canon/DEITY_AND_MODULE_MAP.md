# Pantheon deity and module replacement map

Source snapshot: `958db55b58ddbd6560ce5d0da8f0a6f56488f225` (unpublished post-141 corrective source).
Owner: Pantheon source maintainer. Identity/domain authority:
`docs/DEITY_REGISTRY.md`; source-level display metadata is
`internal/deity/registry.go`. Neither a name nor a source directory proves that
every associated user workflow is complete.

| Responsibility | Primary code / consumed contracts | Replacement and acceptance boundary |
|---|---|---|
| Horus — local instance | `internal/horus`, `internal/dashboard`, native workspace, node-status | Preserve local machine/workspace selection, producer freshness and canonical engine identity; qualify native and retained web consumers |
| Ra — fabric/router | `internal/ra`, `dispatch`, `routerstore`, `routermcp`, `routerboard`, `servicedrain`; `docs/stacklab/ra/canon` | One store/session/claim/evidence authority; preserve wire compatibility, leases, no-replay and retained recovery; Ra owns protocol changes |
| Net / Neith — scope alignment | `internal/neith`, `prd`, `work`, `workstream` | Preserve goal/requirement identity and drift assessment; no new dispatch authority |
| Thoth — continuity | `internal/thoth`, `.thoth`, shipped hook/skill integrations | Preserve session/journal/provenance formats and cross-surface continuation; model memory is not canon |
| Ma'at — quality and System One | `internal/maat` and its casebook/decision/knowledge/knownfail/localreceipt/schedule packages; `docs/stacklab/maat/canon`, `maat-system-one-recipe-v1.json` | Preserve deterministic offline baseline, evidence identity, bounded scans, assessment/case/repair semantics and fresh effect consent; no competing Seshat surface |
| Isis — health/remedy | `internal/isis`, `autoheal`, `guard`, platform adapters | Preserve typed finding-to-effect/postcheck/recovery and protected-work rules; no generic permission to kill/enable services |
| Seshat — compatibility ingestion | `internal/seshat`, Ma'at knowledge adapters | Retain cache/ingestion compatibility only; user-facing knowledge, decisions and fixes belong to Ma'at |
| Anubis — hygiene | `internal/jackal`, `ka`, `cleaner`, `scales`, `reaper`, `trashreceipt`, `mailops` | Preserve scoped review/confirmation, trash-first or irreversible explicit action, exact selected objects and before/after proof |
| Seba — infrastructure/hardware view | `internal/seba`, `platform`, `vitals`, `insight` | Observe qualified hardware/topology data with its owner/freshness; no assumed physical-rail/model qualification |
| Osiris — checkpoints | `internal/osiris`, checkpoint consumers | Preserve uncommitted-work identity and recoverability; never stand in for Ma'at quality acceptance |
| Stele — shared event substrate | `internal/stele`, `ledger`, `oplog` | Preserve event schema/hash/provenance; event liveness is not task completion |
| Apollo — qualified inference producer | `internal/apollo`, `engine`, `gemma`, `modelrouter`, declared worker role and upstream recipe | Consume only qualified producer/ABI/model/performance identities; no inference code/model ownership transfer to PT |
| Mercury / Photon / Hermes | `internal/mercury`, native observer and external wing contracts | Mercury transport surface, Photon hardware, Hermes protocol remain separately owned; no invented link/power/estate metrics or silent rename of historical evidence |
| Shared product infrastructure | Other packages in `MODULE_INVENTORY.json`, native Swift target, scripts/contracts/resources | Named Pantheon maintainer; preserve callers, supported build constraints and recipes; a helper package is not automatically a separate operational workflow |

## Exact module inventory

[`MODULE_INVENTORY.json`](MODULE_INVENTORY.json) enumerates **103** tracked Go packages and **one** Swift target,
**1,026** source/data files and **904** test files at the named
snapshot, with file SHA256 values and direct local Go import literals. Nine
supporting sets enumerate **232** scripts, contracts, configurations, hooks,
extensions, web assets, workflows and brand files. Tests are named, not claimed
executed by this inventory. Direct imports are a static reading aid, not a
compiler-resolved transitive/platform-specific dependency graph. External module
pins remain `go.mod`/`go.sum`; native target/dependency authority is
`macapp/Package.swift`. Platform build tags and actual selected SDK dependencies
must be checked during qualification.

The legacy `cmd/sirsi-menubar`, GUI launchers, optional developer extensions and
non-Mac historical directories are inventoried or explicitly excluded; listing
them does not put them in the commercial payload. Final package inventory decides
what shipped. The final-head receipt must independently verify every enumerated
blob remains identical after the inventory/canon metadata commit. Changes to an
inventoried file require regeneration; historical e26 receipts are retained.

For a single-component upgrade: identify its source and direct callers, read its
recipe/ADR, preserve input/output/evidence schemas and effect authority, run its
focused positive/negative controls and affected caller checks, then rebuild and
qualify the unified release. A package-level change cannot be independently
installed over a signed app or certified from a same-named historical receipt.

The operational process denominator and required logical/data/lifecycle/sequence/
recovery coverage are in `DIAGRAM_INDEX.md`. New behavior requires a mapped
process or explicit addition; an opaque “engine” box cannot hide an unmapped
workflow. Independent semantic review remains required for certification.
