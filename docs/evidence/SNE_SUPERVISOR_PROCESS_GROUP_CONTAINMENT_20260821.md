# SNE Supervisor Process-Group Containment

**Date:** 2026-08-21  
**Status:** M1 COPIED-CANDIDATE LAUNCHD LIFECYCLE ACCEPTED; M5 PACKAGE GATE OPEN

## Defect

Pantheon tracked and signaled only the direct `sned` PID. Context cancellation could terminate that leader before graceful shutdown, while descendants remained outside the cleanup contract. This matches the earlier M1 observation where booting out an old supervisor left a 2.75 GB SNE process alive.

## Repair

- Each admitted SNE launch now owns a dedicated Unix process group.
- Normal stop sends `SIGTERM` to the complete group before canceling the command context.
- Timeout escalation sends `SIGKILL` to the complete group.
- Parent-context loss performs a final group cleanup after the direct child exits.
- Health and memory-ceiling enforcement terminate the same governed group rather than one PID.
- A terminal monitor/restart failure now exits the foreground supervisor command, allowing launchd to perform a fresh, independently admitted replacement instead of leaving a live supervisor with no service child.
- Supervised copied `sned` candidates receive Pantheon's exact PID and monitor parent ownership. Reparenting or parent disappearance triggers local graceful shutdown, covering supervisor `SIGKILL` where parent-side cleanup code cannot run.

No unrelated process is selected by name or pattern. The negative process-group ID is derived only from the exact child Pantheon launched with `Setpgid` enabled.

## Real M1 launchd qualification

The copied-candidate lifecycle gate passed on the M1 (`arm64`, macOS `26.6.2`) with the production memory reserve unchanged:

- normal supervised launch reached readiness;
- killing the service forced the in-process restart to fail closed on real memory headroom, the foreground supervisor exited, and launchd admitted a fresh supervisor/service pair;
- killing that replacement supervisor with `SIGKILL` caused the bound service to observe parent loss and shut itself down;
- launchd admitted a third fresh supervisor/service pair;
- final bootout left the complete test process group empty;
- the pre-existing immutable r5 installation was restored on its original endpoint.

Accepted copied identities:

- supervisor SHA-256: `b20991e80ca25e055798eb86d4540a8ea89e87a6c7ec8d92c3d799eb9db459f3`
- service SHA-256: `37ba074eca7c968e43396d5359b41c78eb73590081ff984b131089dba9928709`
- native runtime SHA-256: `e22d6ae3f92c65a65a3a313946375b5bf4e8669c49f33604df6b1010aa4a8f4e`
- supervisor PIDs: `33648`, `33786`, `33887`
- service PIDs: `33673`, `33806`, `33913`

Durable raw evidence is under `docs/evidence/artifacts/sne-launchd-process-group-m1-20260821/`. The strengthened receipt SHA-256 is `d3ce5589e49eefa557da6cb6c3510e50cb04daaf8704a004f6bd142cf85fd362`.

The first watchdog attempt was invalid because the transient harness retained an older executable at the canonical stage name while the repaired supervisor was copied under a suffixed name. Its child command therefore lacked `--parent-pid`. The admitted rerun replaced the canonical transient executable with the hash-locked repaired binary. Future lifecycle harnesses must bind executable path and expected SHA as one identity; staging a repaired sibling is not deployment.

The harness now requires the expected supervisor SHA before it changes launchd state, verifies the exact executable path selected for the plist, and seals both into the receipt. A deliberate all-zero SHA exited `65`, left installed r5 on the identical PID, and created no test listener. Negative evidence SHA-256 is `4ce0118c792ab095ee0fb24264205e41ee24bf8aec079f9f2eeb505cb8d939b4`.

## Remaining gate

The M5 copied package must independently prove the same lifecycle sequence before cross-device release admission. The M1 result is lifecycle evidence only; it does not prove inference correctness, performance, clean100, or M5 behavior.

The focused integration test creates a real descendant under the admitted child and requires it to be absent after supervised stop. It passed on M5 and then on the reachable M1 (`arm64`, macOS `26.6.2`) using the identical compiled test artifact SHA-256 `2948b46a1b3d1fce187e95336b0f0b20a6a0151bd7767b35a5619ea8694b746e`. The transient M1 test binary was removed after the pass. Model start, launchd bootout, and crash recovery remain separate release gates.
