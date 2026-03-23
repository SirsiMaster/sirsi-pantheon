# ADR-004: Ma'at — QA/QC Governance Agent

## Status
**Accepted** — 2026-03-23

## Context
CI pipelines fail silently. Coverage thresholds exist informally ("cleaner should be 80%+") but are not enforced. Features ship without explicit justification against canon (ADRs, rules, priorities). There is no single system that observations these quality signals, weighs them, and reports a verdict.

Existing tools cover parts of the problem:
- `golangci-lint` catches code style issues but not coverage gaps or canon linkage.
- GitHub Actions runs the pipeline but does not categorize failures or suggest fixes.
- The pre-push hook (session 8) gates individual pushes but does not provide project-wide quality assessment.

None of these tools operate at the *governance* level — asking "should this feature exist?" and "does it meet the standard?"

## Decision
Introduce **Ma'at** (`internal/maat/`) as the QA/QC governance agent for the Sirsi Pantheon.

Ma'at is the Egyptian goddess of truth, justice, balance, and cosmic order. Her feather was the standard against which hearts were weighed. She is not a judge — she IS the standard.

### Agent model
```
observe → assess → weigh → report/act
```

### Scope
| Domain | What Ma'at Weighs | Standard |
|:-------|:-----------------|:---------|
| Canon linkage | Every feature must be justified | ADRs, ANUBIS_RULES, continuation prompt |
| Coverage | Per-module test coverage thresholds | Safety-critical ≥ 80%, all modules ≥ 50% |
| Pipeline | CI run status and failure categorization | Green is the only acceptable state |
| Code quality | Lint, format, vet compliance | golangci-lint clean |

### Core types
- **Verdict**: Pass / Warning / Fail with a Feather weight score (0-100)
- **Assessment**: What was weighed, against what standard, the verdict
- **CanonLink**: Ties a feature to its justification (ADR, rule, priority)
- **Assessor**: Interface for pluggable quality dimensions

### Phased delivery
- **Phase 1 (this session)**: CLI tool — `anubis maat` runs assessments and reports verdicts
- **Phase 2 (future)**: Agent mode — background daemon, quality scoring, release gating

### Prototype role
Ma'at is the first deity to become an autonomous agent. The patterns established here (observe → assess → weigh → report) will be applied to all other deities in future phases.

## Alternatives Considered
1. **Extend Scales** — Add coverage/canon to the existing policy engine. Rejected because Scales weighs *infrastructure metrics* (disk waste, ghost count). Ma'at weighs *development quality* (coverage, canon, pipeline). Different domains deserve separate modules.
2. **External tools only** — Use CodeClimate, SonarQube, or Codecov. Rejected because they don't understand canon linkage, they require SaaS subscriptions, and they break Rule A11 (no telemetry/external services).
3. **CI-only enforcement** — Add checks to GitHub Actions. Rejected because CI runs too late — Ma'at should be available locally via `anubis maat` before pushing.

## Consequences
- **Positive**: Every feature must link to canon. Coverage gaps are surfaced immediately. Pipeline failures are categorized with suggested fixes. Quality is visible and measurable.
- **Positive**: Establishes the agent pattern for the entire Pantheon — all deities can follow Ma'at's observe/assess/weigh/report model.
- **Negative**: Adds a new internal package and CLI command. Increases binary size slightly.
- **Risk**: Over-governance could slow development. Mitigated by making Ma'at advisory in Phase 1 (report only, no blocking).

## References
- [ADR-001](ADR-001-FOUNDING-ARCHITECTURE.md) — Founding architecture (module codenames)
- [ADR-003](ADR-003-BUILD-IN-PUBLIC.md) — Build-in-public process (Ma'at enforces this)
- [ANUBIS_RULES.md](../ANUBIS_RULES.md) — The rules Ma'at verifies canon linkage against
- [CONTINUATION-PROMPT.md](CONTINUATION-PROMPT.md) — Session 9 priority definition
