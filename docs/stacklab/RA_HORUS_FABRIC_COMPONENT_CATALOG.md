# Ra–Horus Fabric — Stack Lab component catalog

**Wing:** `stacklab.wing.ra-horus-fabric`  
**Product:** `sirsi-pantheon`  
**Authority:** Pantheon source; Stack Lab registry pins this record after review.  
**Canonical contract:** `contracts/stacklab/ra-horus-fabric-wing-v1.json`

This catalog is the executable inventory for the Ra/Horus Fabric lane. A
component is not complete merely because its source exists: its listed tests,
security boundary, operational runbook, and traceability row must resolve.

| Component | Source | Tests / proof | Inputs | Outputs | Writes | Safe upgrade rule |
|---|---|---|---|---|---|---|
| Fabric board producer | `internal/ra`, `internal/routerstore`, `internal/ledger` | package tests; `docs/router-service/stacklab/WING.md` contract | router service receipts, ledger tasks, lane state | `FabricBoard` classifications | none in read path | preserve one producer and stream-separated counts |
| Native Ra handoff | `macapp/Sources/SirsiMenubar/SirsiEngine.swift`, `Views.swift` | Swift build; CLI flag checks; identity tests | typed destination, canonical Horus sender | one authenticated handoff request | router service only through bounded command | sender is fixed to `horus`; callers cannot impersonate it |
| Router CLI | `cmd/sirsi/routercmd.go`, `cmd/sirsi/ra.go` | `cmd/sirsi` tests; CLI help/invalid-input tests | explicit operator command | JSON/human router views and typed actions | declared router backend | never reopen a local ledger after service cut-over |
| Router service boundary | `internal/router`, `internal/routerstore` | router service and rollback fixtures | authenticated session, host-bound token | leases, receipts, wake events | canonical service backend | preserve token host binding, TTL fencing, 503 outage behavior |
| Maat projection consumer | `internal/maat/casebook`, dashboard endpoints | Maat recipe tests | Ma’at decision receipts | read-only cases and determinations | local journal only through approved writer | never turn a projection into authorization |
| Horus dashboard surface | `internal/dashboard`, `cmd/sirsi/dashboard.go`, `cmd/sirsi/dashboardrouter.go` | dashboard tests and endpoint contract checks; `TestAPIRouterNilFailingAndWired`, `TestRouterViewIsReachableFromTheSPA`, `TestReadChangelogReleases` | `FabricBoard`; lane ping verdicts; queue; consumer cap; registry pin; known-failure catalog; swap receipt; CHANGELOG | Fleet view (retired aliases hidden) and the Router view (what each release added, known failures, lane verdicts, queue) via `GET /api/router` | none | a producer error is a 5xx, never an empty panel; every new router capability adds its section in the same change |
| Known-failure catalog | `internal/maat/knownfail`, `cmd/sirsi/maatknownfail.go`, wake-loop recognition in `internal/router/wake.go` | `TestEveryGuardTestExistsInTheRepo`, `TestRegisterThenResolveRequiresARealGuard`, `TestParseRejectsAResolvedEntryWithoutAGuard` | failure signatures, fix versions, regression guard test names | recognized failure class with its fix in lane state and log | `internal/maat/knownfail/catalog.json` through a PR | a resolved entry without a guard test that exists is rejected by the loader and CI |
| Registry pin and re-sync | `internal/router/registrysnapshot.go`, `cmd/sirsi/routerregistrycmd.go` | `TestRegistryReadsOriginSnapshotOnceSynced`, `TestRegistrySyncRefusesBadOriginAndStaleSnapshotIsIgnored` | origin/main `agents.json` | host registry snapshot; refusal of working-tree identity writes | `~/.sirsi/registry` | validate before replace; atomic write; snapshot scoped to its router root |
| Swap and memory hygiene | `internal/swaphygiene`, `cmd/sirsi/swaphygiene.go` | `TestAssessDistinguishesIdleAllocationFromPaging`, `TestAssessNeverInventsAHealthyOrSickReading` | `vm.swapusage`, `vm_stat`, `memory_pressure` | receipts with correctness-only and release-timing flags; restart proposal only | `~/.sirsi/swap-hygiene` | allocation is not paging; never touches swap files or restarts a host |
| Claim eligibility and consumer cap | `internal/routerstore/taskeligibility.go`, `internal/router/consumerslots.go` | `TestTaskEligibilityExplainsEveryRefusal`, `TestConsumerSlotsCapHostConcurrency` | task rows, consumer pid markers | `router task why`; host-wide consumer cap | none | the diagnosis must agree with ClaimTask; the cap holds, never drops, a lane |
| Stack Lab contract | `contracts/stacklab/ra-horus-fabric-wing-v1.json`, this catalog, recipe | `sirsi stacklab doctor`; schema validation | exact source and receipt links | provenance record | contract files only | registry pin must match bytes on origin/main |

## Authority boundaries

- Ra owns routing and durable handoff semantics; Horus owns the local system
  view; Ma’at judges evidence and contention; Photon is hardware only; Apollo
  and Apollo Flash are inference profiles; Mercury is the transport protocol.
- The lane has one router authority. Local SQLite/router files are not a
  second live store after service cut-over.
- Cross-lane communication is receipt-only. No lane receives arbitrary
  filesystem authority from this catalog.

## Completion rule

The companion traceability matrix is the release checklist. A row marked
`OPEN` is a real work item, not a green claim. The Stack Lab doctor must pass
and the record must be byte-pinned by the universal registry before the wing
is called canonical.

**Diagrams:** every process this catalog covers is indexed in [RA_HORUS_FABRIC_DIAGRAM_INDEX.md](RA_HORUS_FABRIC_DIAGRAM_INDEX.md) (SL-DIAGRAM-001).
