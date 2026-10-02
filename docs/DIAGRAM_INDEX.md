# Diagram coverage index — sirsi-pantheon

Standing requirement: [SL-DIAGRAM-001](https://github.com/SirsiMaster/sirsi-stacklab/blob/main/docs/PROCESS_DIAGRAM_RUBRIC.md) (canonical copy: `sirsi-stacklab/docs/PROCESS_DIAGRAM_RUBRIC.md`).
Owner: claude-pantheon. Source revision: `6b9d55ca` (2026-10-02). Last audit: 2026-10-02.
Inventory basis: `PANTHEON_RULES.md` deity/module table, `docs/ADR-INDEX.md`, `internal/` package list (73 packages), `.github/workflows/ci.yml`.
Coverage: **0 / 20 processes have SL-DIAGRAM-001-compliant coverage** (logical + data + state/sequence/recovery views with traceability). 8 pre-existing Mermaid diagrams in `docs/diagrams/` give **overview-only** coverage for 6 of the 20 rows — they predate this rubric, are marketing/architecture-pitch diagrams (single logical view, no data/state/recovery views, no process IDs), and do **not** satisfy the requirement on their own per the rubric's explicit rule that "a single high-level architecture chart does not satisfy all processes."

This is a **Phase 1 adoption + inventory audit**, not completed diagram authorship. Per the rubric's existing-work rule: audit coverage, retain qualifying diagrams, create owned backlog rows for every gap, prioritize active user/release paths. No row below is closed. No mechanical completeness check exists yet (see "Checks" section at the bottom) — this document only proves enumeration, not per-process diagram correctness.

| Process ID / name | Requirements | Trigger, inputs → outputs | Owner | Logical view | Data view | State / sequence / recovery | Source / state | Evidence / last review | Gap / next action |
|---|---|---|---|---|---|---|---|---|---|
| P-001 / Jackal local scan+clean | ADR-001, `docs/SCAN_RULE_GUIDE.md` | CLI invoke → scan rule set → findings → (dry-run) clean | claude-pantheon | `docs/diagrams/08-scan-pipeline.mmd` (overview only) | none | none | implemented | 2026-10-02 inventory only | OPEN: author data view (rule → finding payload → safety.go gate) and recovery view (dry-run abort, protected-path reject) |
| P-002 / Ka ghost detection | ADR-002 | periodic/manual scan → residual-app match → findings | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram exists; full SL-DIAGRAM-001 set needed |
| P-003 / Scarab fleet sweep | Roadmap Phase 4, `docs/ARCHITECTURE_DESIGN.md` | `--confirm-network` opt-in → subnet discovery → agent dispatch | claude-pantheon | `docs/diagrams/06-fleet-architecture.mmd` (overview only) | none | none | partial/roadmap | 2026-10-02 inventory only | OPEN: confirm current implementation state before diagramming (roadmap vs shipped) |
| P-004 / Scales policy engine | Roadmap Phase 6 | policy load → findings evaluation → enforcement verdict | claude-pantheon | none | none | none | roadmap/not yet implemented | 2026-10-02 inventory only | OPEN: defer diagram until implementation lands; do not draw an invented path |
| P-005 / Hapi resource governance (guard) | ADR-006, ADR-031-A/B/C, ADR-040 | RSS sample → `LoadBearingPIDs` check → right-size/kill decision | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: highest-priority gap — this is the exact class of process A35 flagged (capacity caps, load-bearing exemptions) as having shipped with scope-mismatched checks before |
| P-006 / Router Tier-0 dispatch | ADR-036, ADR-049, ADR-052, ADR-053 | inbox item arrives → `WakePass` → claim/lease → ack/close | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: core A27/A29 invariant; no diagram covers wake/lease/ack lifecycle |
| P-007 / Thread registry, census, workboard | ADR-050, A33 | process discovery/census sweep → registry row → workboard read-model | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram; this is the primitive A33 added after the unregistered-broker incident |
| P-008 / Ma'at QA gate (pre-push + CI) | ADR-004, A6, A28 | `git push` → `.githooks/pre-push` → lint/vet/test → pass/block | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: diagram arm/disarm states (A28's "shipped vs armed" distinction) explicitly |
| P-009 / Orchestration Brain tiering | ADR-034, A29, A30 | role lookup → `brain.yaml` config → provider dispatch (none/local/hosted) | claude-pantheon | none | none | none | implemented (P1b) | 2026-10-02 inventory only | OPEN: diagram the Level 0-3 provider resolution and RAM-gate consult path |
| P-010 / Horus code graph + ops dashboard | ADR-017, ADR-026 | file watch → graph index → dashboard render / CLI query | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram |
| P-011 / Menubar app | ADR-010, ADR-030 | launch → stats poll → popover render | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram |
| P-012 / Dashboard / SNE | `internal/dashboard`, `internal/sne` | HTTP request → auth/identity projection → SPA/API response | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram; identity/auth boundary is exactly the trust-boundary view the rubric requires |
| P-013 / Thoth memory | ADR-058, ADR-059 | significant change → `.thoth/memory.yaml` + `journal.md` update | claude-pantheon | `docs/diagrams/04-thoth-memory.mmd` (overview only) | none | none | implemented | 2026-10-02 inventory only | OPEN: author data view (what gets written/read, residency) |
| P-014 / Reaper + supervision | ADR-043, ADR-057, ADR-061 | supervisor sweep → stray/stranded detection → reap or escalate | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram; recovery/escalation path is the core of this process |
| P-015 / Gemma broker (local model serving) | ADR-031, ADR-045, ADR-046, A32 | `sirsi gemma serve` → model load → inference request → response | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: load-bearing PID recognition + resize-not-kill path needs its own recovery view (direct descendant of the A32 incident) |
| P-016 / CI/CD pipeline | `.github/workflows/ci.yml`, A6 | push/PR → lint/test/build → status check → merge gate | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram |
| P-017 / Mailops | ADR-062 | inbound mail event → routing rule → action | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram |
| P-018 / Package inventory | `cmd/sirsi-package-inventory` | scan trigger → package enumeration → report | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram |
| P-019 / Self-update / updater | `internal/updater`, `internal/selfupdate`, ADR-023 | version check → binary fetch → verify → swap | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram; rollback/failed-verify path is a required recovery view |
| P-020 / Router v2 durable dispatch (store) | ADR-036 | item write → durable store → lease/claim → ack/close transitions | claude-pantheon | none | none | none | implemented | 2026-10-02 inventory only | OPEN: no diagram; this is the state-machine view the rubric calls out explicitly (admitted/running/failed/recovery) |

Unlisted processes (the remaining ~53 `internal/` packages not named above — e.g. `tui`, `vault`, `stele`, `neith`, `isis`, `osiris`, `seba`, `rtk`, `vitals`, `yield`, `deity`, `provider`, `mcp`, `snemodels`, `stealth`, `oplog`, `reason`, `report`, `suggest`, `profile`, `workstream`, `work`, `canon`, `ignore`, `platform`, `help`, `output`, `logging`, `notify`, `version`, `liveness`, `localrouter`, `modelrouter`, `routercfg`, `routerstore`, `routerboard`, `seshat`, `setup`, `sight`, `apprecovery`, `autoheal`, `agentguard`, `dispatch`, `engine`, `govern`, `insight`, `brand`) are **gaps by the rubric's own rule**: "Unlisted processes count as gaps; a partial inventory cannot produce 100% coverage." Denominator for future coverage reporting is the full package/process set, not just these 20 — this file tracks the 20 highest-traffic, most-cited-in-canon processes first.

## Diagram record

- Stable diagram ID and covered process IDs: none assigned yet — pending diagram authorship per row above.
- Editable source and rendered/viewable location: `docs/diagrams/*.mmd` (6 pre-existing overview diagrams, not rubric-compliant) + `docs/diagrams/rendered/` (SVG).
- Plain-language purpose and reading guide: see `docs/diagrams/README.md` (brand/style only, not a rubric reading guide).
- Component/interface/store/actor identities: not yet mapped per-process.
- Logical branch and data payload labels: not yet mapped per-process.
- Boundaries, failure/retry/recovery paths: not yet mapped per-process — flagged as the largest gap (every row above lists "none" for state/sequence/recovery).
- Source/model/runtime/recipe identity where relevant: see per-row "Source / state" column.
- Reviewer, review date, known gaps and next action: claude-pantheon, 2026-10-02, see per-row "Gap / next action".

## Checks — what they do and do not prove

**No mechanical completeness check exists in this repo yet.** The rubric explicitly permits deferring CI/Ma'at enforcement ("No automatic Ma'at/CI enforcement is claimed by this document... Pantheon/Ma'at owns any subsequent admission/CI enforcement") and explicitly warns against overclaiming what a check proves (Rule A35, this repo's own canon): "Schema/file checks establish shape/existence only; they cannot prove semantic completeness."

If a future check is added (e.g., a CI step verifying every row in this table has non-"none" logical/data view links before a release tag), it would prove:
- **Does prove**: every enumerated process has a non-empty diagram reference on file.

It would **not** prove:
- Whether the referenced diagram is accurate, current, or matches observed runtime behavior.
- Whether the process inventory itself is complete (the 53 unlisted packages above are a known, named gap).
- Whether failure/recovery paths shown are real vs. invented (that requires human review against source, per the rubric's review checklist).

Recommendation, not yet implemented: a `go test` or CI step that fails if `docs/DIAGRAM_INDEX.md` contains a row with `Gap / next action` unresolved past a release tag boundary — scoped narrowly to existence/shape, documented with the same caveat above, so it cannot be cited as proof of semantic completeness.

## Adoption record

- Adopted 2026-10-02 in response to router item `20261002-145023-codex-apollo-claude-pantheon-standing-owner-rubric-diagram-every-process-in-every-wing` (decision, from codex-apollo).
- This file is the traceability matrix required by SL-DIAGRAM-001 for sirsi-pantheon. Future diagram authorship updates both the diagram source and the matching row here in the same change, per the rubric's maintenance rule.
