### Added
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — the Rule-17 sprint
  plan SSA's build authorization (item `20261007-144957`) required before any
  ADR-065 informer code: Commercialization Gate section, a `/goal` naming the
  single-host informer + `AgentConfig.Delivery` field, the delivery/read-ack
  distinction, token-host validation (live, non-revoked) against the ADR-067
  adoption record, the six required negative controls (five SSA-named
  properties plus the coexistence/zero-duplicate-spawn control) with named
  tests and red-before-green evidence requirements, the shared per-lane
  admission-state design for coexistence with `RunWakeLoop`, and a rollback
  contract. No implementation lands in this change; Phase 1 (code) is a
  separate work item under the same `rs-37` ledger entry.

### Fixed
- `docs/ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md` and
  `docs/ADR-INDEX.md` — corrected the sprint header's misattribution of SSA
  authorization item `20261007-144957` to `sirsi-hardware-admin` (it was
  `sirsi-software-admin`); marked the stale "blocked-by rs-36" / "SSA verdict
  and owner bind still pending" status text as historical/superseded and
  recorded the corrected dependency (`rs-36` awaits `rs-37` Decision 4, not
  the reverse); annotated ADR-065 Decision 2's original informer-sets-`read_at`
  text as superseded for Phase 1 by the SSA-corrected delivery-attempt/read-ack
  distinction.

Refs: PANTHEON_RULES.md A23/A26 (Rule-17 sprint planning), A35 (scope the check to the claim), docs/ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md, ledger rs-37-adr065-router-owned-informer
