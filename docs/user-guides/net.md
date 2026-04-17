# Net — Scope Weaver

Net (formerly Neith) defines the task scopes that Ra dispatches, tracks plan alignment against build logs, and validates cross-module consistency.

## Commands

### Check plan alignment
```bash
sirsi net status              # Score alignment against BUILD_LOG.md
```

Loads your project's BUILD_LOG.md and scores how well the build log matches the development plan. Returns an alignment percentage and verdict (ALIGNED or DRIFTING).

### Validate module consistency
```bash
sirsi net align               # Run real checks across all modules
```

Performs 5 checks:
1. **Ma'at**: `go vet ./...` passes
2. **Anubis**: `go build ./...` succeeds
3. **Hygiene**: `gofmt` reports no violations
4. **Thoth**: `.thoth/memory.yaml` is present
5. **Isis**: System health assumed (network checks are separate)

If any check fails, reports exactly which deity's domain has issues.

## How Net Works with Ra

1. Net defines **scopes** (YAML configs in `configs/scopes/`)
2. Ra reads those scopes and deploys agents
3. Agents execute the scope tasks autonomously
4. Net can then verify alignment post-execution

Net sets the plan. Ra executes it. Ma'at judges the results.
