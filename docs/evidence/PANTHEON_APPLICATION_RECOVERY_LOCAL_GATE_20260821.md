# Pantheon Application Recovery Local Gate

Date: 2026-08-21

## Verdict

Accepted for the isolated macOS checkpoint-aware process mechanism. This is not yet clean-host qualification for arbitrary `.app` bundles or launchd services.

## Proven behavior

- A private copied helper executable was launched as a real macOS process.
- `restore` captured and verified a declared checkpoint file, stopped the original process, launched the registered executable, required a distinct replacement PID, and persisted `ready`.
- `fresh` stopped and verified exit of the original process, removed exactly one declared transient file, launched the registered executable, required a distinct replacement PID, and persisted `ready`.
- The helper and all state lived in the test-private temporary directory. No SNE, Codex, browser, user application, or launchd service was restarted.
- A uniquely named temporary user LaunchAgent then passed a real `launchctl kickstart -k` supervised replacement with a distinct PID and was booted out during cleanup. No installed Sirsi or user service was touched.
- A temporary background `.app` consumed its own persisted session, was gracefully quit by bundle identity, relaunched by exact registered bundle path, received a distinct PID, and consumed the same session again.
- A newly constructed manager resumed a durable `stopped` receipt and completed replacement without access to the prior manager's memory.

## Command and result

```text
go test ./internal/apprecovery -run 'TestDarwinCheckpointRestoreUsesRealReplacementProcess|TestDarwinFreshRestartClearsDeclaredTransientFile|TestRecoveryHelperProcess' -count=1 -v

TestRecoveryHelperProcess                              PASS
TestDarwinCheckpointRestoreUsesRealReplacementProcess PASS
TestDarwinFreshRestartClearsDeclaredTransientFile      PASS
package result                                         PASS (0.322s)
```

The separate launchd fixture gate passed:

```text
TestRecoveryHelperProcess                    PASS
TestDarwinLaunchdFreshUsesSupervisedReplacement PASS
package result                               PASS (10.314s)
```

The 10-second wall time includes temporary launchd bootstrap and cleanup. It is not an application-recovery latency claim.

After the exact-path corrections, the complete focused gate passed:

```text
go test ./internal/apprecovery ./internal/dashboard -count=1
internal/apprecovery PASS (11.238s)
internal/dashboard   PASS (2.713s)
```

## Failure-derived rules

- macOS may report an app launched through LaunchServices under `/private/var/...` while a directly executed process retains `/var/...`. PID identity therefore admits only the exact registered path or its exact filesystem-canonical equivalent.
- BSD `pgrep` uses POSIX regular expressions and does not support non-capturing groups. Exact alternative paths use a standard capturing alternation.
- App relaunch by bundle-ID lookup can depend on mutable LaunchServices registration. Pantheon opens the exact `.app` path derived from the registered executable and uses bundle identity only for graceful quit.

The broader focused gate also passed immediately before this proof:

```text
go test ./internal/apprecovery ./internal/dashboard
internal/apprecovery PASS (0.217s)
internal/dashboard   PASS (2.832s)
```

## Remaining gates

- registered user application qualification beyond the isolated `.app` fixture;
- registered user application fresh restart beyond the isolated fixture;
- launchd persistent service-specific state recovery (generic supervised replacement is proven);
- complete Pantheon process replacement/reboot resume beyond the new-manager receipt proof;
- sleep/wake and reboot persistence;
- M1 and M5 clean-host proof;
- hands-on accessibility verification of the Recovery view.

## M1 clean-host attempt

The authorized M1 initially answered read-only SSH as `arm64` on macOS `26.6.1`; neither a Pantheon repository nor recovery registry was present. SCP then closed before transfer, and the SSH-stream fallback timed out before any bytes or test execution were admitted. The M5 temporary test artifact was removed. This is an unavailable-host result, not M1 evidence.

A later bounded read-only SSH retry also timed out. The M1 gate remains pending; no repeated polling loop was left running.

## Support diagnostics

Pantheon's privacy-safe SNE diagnostics now include the governed recovery target ID, class, supported modes, auto-resume policy, latest mode/phase, and stable failure code. Executable/state paths, snapshot hashes, arguments, PIDs, and raw driver errors remain excluded. The combined dashboard/recovery gate passes after this schema extension.

Google Workspace owner-readable mirroring is pending connector synchronization. The canonical repository source and Desktop owner mirror are present.
