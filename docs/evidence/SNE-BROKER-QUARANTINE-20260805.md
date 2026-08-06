# SNE broker quarantine — Pantheon repair evidence

## Incident contract

The production SNE broker's MLX active memory rose from 56.39 GB to 63.67 GB
while idle on a 48 GB host. Booting out only the KeepAlive broker did not hold:
the resident Horus supervisor's gemma-liveness duty started it again. The
read-only `ai.sirsi.liveness-watch` was initially blamed, but source and its own
run log prove it only probes and routes; disabling it would remove observation
without stopping restoration.

## Repair

Pantheon now exposes a broker-specific, reversible operator state:

```text
sirsi gemma serve --quarantine
sirsi gemma serve --status
sirsi gemma serve --restore
```

Quarantine renames only
`~/Library/LaunchAgents/ai.sirsi.gemma-broker.plist` to the existing
`.plist.quarantined` convention, disables and boots out only that broker label,
waits until launchd no longer reports it loaded, and clears stale PID/port
files. This is sufficient without disabling Horus or any watcher:

- dead-label recovery ignores files not ending in `.plist`;
- gemma-liveness invokes the normal `sirsi gemma serve`, which refuses when the
  canonical installed plist is absent;
- normal `sirsi gemma serve` explicitly refuses a quarantine bypass; and
- `sirsi setup` preserves the quarantined artifact instead of recreating a
  canonical plist.

Restore is explicit and fail closed. It enables the broker label, renames the
definition back, bootstraps it, and requires normal readiness. Bootstrap or
readiness failure returns the definition to quarantine.

## Verification

- focused quarantine/restore CLI tests: PASS;
- installer quarantine-preservation test: PASS;
- conflicting canonical/quarantine definitions fail visibly: PASS;
- readiness failure automatically re-quarantines the candidate: PASS;
- existing native-broker invariants: PASS;
- gemma-liveness/dead-label focused regressions: PASS;
- `go build ./cmd/sirsi`: PASS;
- `go vet ./...`: PASS;
- `go test ./...`: PASS, including `cmd/sirsi` (76.631s) and
  `internal/setup` (26.992s);
- `git diff --check`: PASS.

The tests stub launchctl and do not alter live services. This evidence does not
claim the P0 SNE memory repair is numerically closed; it establishes only that
Pantheon can hold a rejected broker candidate safely offline while keeping CTR,
Horus, and liveness observation active.
