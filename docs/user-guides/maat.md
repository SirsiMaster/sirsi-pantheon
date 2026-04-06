# Ma'at — Quality Gate

Ma'at is the sole authority on code quality across the Pantheon. She runs governance audits, enforces policies, measures coverage, and provides quality scores.

## Commands

### Full governance audit
```bash
pantheon maat audit              # Full assessment with streaming progress
pantheon maat audit --skip-test  # Skip test execution (faster)
```

Checks: code coverage, formatting (gofmt), static analysis (go vet), canon compliance, and dependency health. Streams per-package progress so you see what's happening.

### Policy enforcement
```bash
pantheon maat scales             # Detect infrastructure policy drifts
pantheon maat scales --fix       # Auto-fix detected drifts
```

Enforces YAML-defined policies for code standards, dependency versions, and project structure.

### Autonomous healing
```bash
pantheon maat heal               # Auto-remediate governance failures
pantheon maat heal --fix --full  # Full remediation pass
```

Delegates to Isis for actual fixes: formatting, linting, vetting, and coverage gaps.

### Dynamic measurement
```bash
pantheon maat pulse              # Real-time metrics snapshot
pantheon maat pulse --json       # JSON output to .pantheon/metrics.json
```

Captures current quality metrics for dashboards and CI integration.

## Pre-Push Gate

Ma'at runs automatically on every `git push` via the pre-push hook:

```
𓆄 Ma'at pre-push gate... [Pantheon | depth: fast]
```

Three depth tiers:
- **fast** (default): gofmt + go vet + build + tests on changed packages (~10-30s)
- **standard**: fast + Ma'at coverage + canon check (`MAAT_DEPTH=standard git push`)
- **deep**: full test suite + full Ma'at assessment (`MAAT_DEPTH=deep git push`)

## Feather Weight Score

Ma'at scores projects on a 0-100 scale. A module scoring below 85 is considered "not yet canon" and cannot be included in a stable release.

## Output
```bash
pantheon maat audit --json       # JSON audit report
pantheon maat pulse --json       # JSON metrics snapshot
```
