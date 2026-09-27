# Ra–Horus Fabric — lane completion and traceability matrix

**Lane:** `stacklab.wing.ra-horus-fabric`  
**Last reviewed:** 2026-09-27  
**Status:** source candidate; release promotion remains evidence-bound.

This is the single lane-level index for the required product documents. Each
row names the authoritative artifact, implementation surface, and proof still
needed. It is intentionally honest about runtime/package/registry work that
has not been proven.

| Requirement | Authoritative artifact | Implementation / acceptance evidence | State | Next action |
|---|---|---|---|---|
| PRD / product promise | `docs/prd/ROUTER_V2_DURABLE_DISPATCH.md`, `docs/prd/SIRSI_V2_APPLICATION.md` | Fabric board, durable dispatch, one-engine surface | PARTIAL | reconcile remaining user-facing Ra stories with the board |
| Development plan | `docs/SURFACE_REWRITE_MASTER_PLAN.md`, `docs/QA_PLAN.md` | ordered component recipes and Ma’at gates | PARTIAL | attach each open row to a router task and receipt |
| User stories | `docs/TRACEABILITY.md` deity/surface matrix | Ra fleet, handoff, dashboard, operator stories | PARTIAL | add acceptance examples for live message/review/return |
| Security and threat model | `docs/SAFETY_DESIGN.md`, `docs/runbooks/router-service-tokens-and-rollback.md` | default-deny roots, host-bound tokens, TTL/revocation, rollback | ACCEPTED SOURCE | run installed-service and outage rehearsal on release candidate |
| Interaction / system design | `docs/design/FABRIC_SURFACE_UNIFICATION.md`, `docs/router-service/stacklab/WING.md` | one `FabricBoard`, stream-separated counts, stale-data behavior | ACCEPTED SOURCE | preserve contract while completing remaining surfaces |
| Production runbook | `docs/runbooks/router-service-tokens-and-rollback.md` | token rotation, cut-over, rollback, outage behavior | ACCEPTED SOURCE | attach latest live rehearsal receipt |
| Changelog | `CHANGELOG.md` | current RA identity fix and release entry | ACCEPTED SOURCE | update for each promoted release candidate |
| Traceability matrix | this file | every requirement mapped to source and proof | ACCEPTED SOURCE | keep rows current with every change |
| Stack Lab component catalog | `docs/stacklab/RA_HORUS_FABRIC_COMPONENT_CATALOG.md`, `contracts/stacklab/ra-horus-fabric-recipe-v1.json` | source/test/input/output/write inventory | ACCEPTED SOURCE | pin catalog and recipe in registry |
| Native sender identity | `macapp/Sources/SirsiMenubar/SirsiEngine.swift` | sender fixed to `horus`; caller cannot choose `--from` | ACCEPTED SOURCE | release build and runtime handoff proof |
| One router authority | `internal/routerstore`, `docs/runbooks/router-service-tokens-and-rollback.md` | service env refusal of local fallback | ACCEPTED SOURCE | verify both installed Macs after cut-over |
| Release readiness | `.github/workflows/release.yml`, `.goreleaser.yaml` | build/package/sign/notarize receipts | OPEN | repair platform-specific CI skips, then run protected release workflow |
| Stack Lab registry authority | `contracts/stacklab/v2/PROVENANCE.md` | universal registry byte pin | OPEN | publish accepted contract bytes to registry |

## Definition of done

The lane is clear only when every row is either `ACCEPTED` with a durable
receipt or explicitly assigned to an open router item. Source-only acceptance
does not imply installed runtime, signed package, notarization, or production
promotion.
