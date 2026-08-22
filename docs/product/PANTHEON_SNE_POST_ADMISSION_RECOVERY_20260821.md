# Pantheon SNE Post-Admission Recovery

Date: 2026-08-21

## Decision

Pantheon now treats SNE process liveness and SNE execution readiness as separate facts. A process that remains alive after a fatal native execution failure is not allowed to remain advertised as usable.

The canonical recovery path remains Pantheon's registered SNE supervisor. Sirsi Hardware Admin does not provide a separate restart API, and no duplicate restart service was introduced.

## Runtime Contract

1. Startup health polling cannot cause a restart.
2. `WaitReady` admits only a process generation whose complete runtime, model, manifest, serving-policy, cache, and session identity matches the registered launch contract.
3. Post-admission health monitoring is scoped to that exact process generation.
4. One failed probe is treated as transient. Three consecutive readiness or identity failures are required in production.
5. The watcher signals only the currently admitted registered child.
6. The existing supervisor performs a fresh-process replacement after resource and memory admission.
7. The replacement is untrusted until it independently passes exact readiness admission.
8. This mechanism does not reboot the host, bypass FileVault, restart arbitrary processes, or silently change model, precision, runtime, framework, or execution mode.

## Evidence

Focused forced-failure proof:

```text
go test ./internal/sne -run 'TestSupervisor(ReplacesAdmittedProcessAfterConsecutiveReadinessFailures|RestartLaunchesFreshProcess)$' -count=1
ok github.com/SirsiMaster/sirsi-pantheon/internal/sne 2.375s
```

Integrated Pantheon proof:

```text
go test ./internal/sne ./internal/dashboard ./cmd/sirsi ./cmd/sirsi-gui ./cmd/sirsi-menubar -count=1
PASS
```

The forced-failure test proves that Pantheon admits process A, observes repeated post-admission readiness failure, replaces it exactly once, obtains a distinct process B, and requires process B to pass exact readiness before returning success.

## Remaining Release Gate

This source-level and process-level proof does not replace clean-host model-backed qualification. The copied SNE readiness-latch package must still be exercised on unlocked clean M1 and M5 sessions with a real successful stream, a reproducible fatal native failure, observed readiness rejection, supervised recovery, and a successful post-recovery stream. Performance claims remain unchanged until those gates pass.

## Human Access

The owner-readable HTML mirror is stored in the Desktop Sirsi Owner Reading Room. A native Sirsi Google Workspace mirror remains required; its absence is a visible publication blocker, not permission to treat the record as fully published.
