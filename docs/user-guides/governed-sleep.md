# Governed sleep: source qualification status

This work adds the mechanisms for a bounded sleep-and-return transaction. It is not yet an installed operator command or permission to sleep a machine.

The controller must first establish an existing enrolled authority specifically permitted to authorize sleep, the actual Mac and boot identity, current workload-owner clearance, an independent observer and an exact scheduled wake. Missing or unknown inputs deny the operation. Existing remote-desktop access, a cloud scheduler entry or an earlier quiet window does not provide that authority.

The transaction keeps durable, single-use records. A failed or uncertain operation must be inspected through its original request; do not repeat it with another request ID. Native command success does not prove the Mac slept or returned safely. Independent full-wake and workload-continuity evidence is required.

Source tests use generated test keys and fake native command execution. They never sleep the Mac, schedule a real wake, change a setting, enroll a key or alter the installed guard. The current source packet is not release-ready as a complete operator workflow: CLI and real authority/custodian/observer enrollment remain explicit integration work.

See the implementation plan and ADR-119 for precise scope. Actual shutdown, reboot, FileVault changes and unattended cold unlock are not part of this feature.
