### Added
- `docs/sprints/SPRINT-ADR065-ROUTER-INFORMER-PHASE1.md` — the Rule-17 sprint
  plan SSA's build authorization (item `20261007-144957`) required before any
  ADR-065 informer code: product classification, a `/goal` naming the single-
  host informer + `AgentConfig.Delivery` field, the delivery/read-ack
  distinction, token-host validation against the ADR-067 adoption record, and
  five required negative controls. No implementation lands in this change;
  Phase 1 (code) is a separate work item under the same `rs-37` ledger entry.

Refs: PANTHEON_RULES.md A23/A26 (Rule-17 sprint planning), docs/ADR-065-ROUTER-OWNED-INFORMER-LANES-CARRY-NO-ARMING-LOGIC.md, ledger rs-37-adr065-router-owned-informer
