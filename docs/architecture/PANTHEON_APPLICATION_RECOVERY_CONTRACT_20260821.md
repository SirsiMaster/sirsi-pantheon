# Pantheon Application Recovery Contract

Date: 2026-08-21

## Product promise

Pantheon can restart a registered application or service and continue its governed workflow from durable application state. This is the same class of behavior users recognize when Chrome reopens windows and tabs after relaunch.

Pantheon does not claim that macOS can serialize and restore an arbitrary process instruction pointer, GPU command stream, open socket, or unpersisted heap through public APIs.

Every restart has an explicit intent:

- `restore`: preserve and verify declared application session or checkpoint state before relaunch.
- `fresh`: deliberately discard only pre-registered transient queue/cache files, then relaunch from a clean state.

The modes are never inferred. A receipt records the selected mode, and a target without declared restore state cannot accept `restore`.

## Supported recovery classes

1. `app_saved_state`: the application owns durable session state. Pantheon verifies declared state files, requests a normal quit, relaunches the bundle, requires a replacement PID, and verifies readiness.
2. `launchd_service`: launchd owns process supervision. Pantheon performs a governed `kickstart -k`, requires a replacement PID, and verifies readiness.
3. `checkpointed_process`: the process exposes a declared durable checkpoint contract. Pantheon verifies the checkpoint before launching the registered executable.
4. `unsupported`: targets without a durable state contract fail closed. Pantheon never pretends that a restart preserved work.

## Durable sequence

Every operation persists `pantheon.app-recovery.v1` receipts atomically after capture, stop, start, and readiness. If Pantheon itself exits after capture or stop, `Resume` continues from that durable phase rather than repeating an unsafe stop.

Admission requires:

- a pre-registered target identity;
- an exact recovery class;
- declared state paths for stateful recovery;
- an exact executable and platform identity;
- an old process identity where present;
- a new, different PID after relaunch;
- an optional loopback-only HTTP readiness probe;
- a final receipt with no raw environment, command output, user content, or credentials.

Fresh-state deletion is constrained to exact registered absolute file paths. Symlinks and directories are rejected, preventing broad recursive cleanup from entering the recovery surface.
For ordinary applications and checkpoint-aware processes, Pantheon verifies that the old PID has exited before clearing those files. A generic launchd restart clears process-owned in-memory queues and caches through supervised replacement; persistent launchd state requires a service-specific declared contract rather than opportunistic deletion.

## Security boundary

Recovery is registry-driven, not a general command-execution endpoint. Bundle IDs and launchd targets are validated, executable paths must be absolute, readiness is loopback-only, and Pantheon does not kill unknown PIDs or accept shell fragments.

## Product integration

Pantheon loads `~/.config/sirsi/recovery-targets.json` only when it is a regular, current-user-owned file inaccessible to group and other users. The CLI and resident menubar dashboards use the same registry and receipt directory. The dedicated Recovery view exposes only target identity, class, supported modes, phase, and privacy-safe failure code.

The example registry is `configs/recovery-targets.example.json`. It is documentation, not an enabled target.

Normal enrollment uses `sirsi recovery add`; operators do not need to hand-edit the registry. `sirsi recovery list` exposes capability identities without paths, and `sirsi recovery remove TARGET_ID` atomically removes authority. Pantheon must be restarted after registry mutation so a running process cannot acquire new authority silently.

`--auto-resume` is opt-in. On startup Pantheon may continue only an already durable `captured`, `stopped`, or `started` receipt for that target. It never initiates a new restore or fresh restart, and it ignores ready, failed, absent, and non-opted-in receipts.

Enrollment admits only an existing non-symlink executable file, valid platform identity, absolute non-symlink regular durable-state files, optional absent-or-regular transient files, and loopback HTTP readiness. Registry mutation rechecks current-user ownership and private permissions before every atomic update.

Application relaunch uses the exact `.app` bundle derived from the registered executable path, not a mutable LaunchServices bundle-ID lookup. Bundle identity remains the graceful-quit authority.
Process identity accepts only the exact registered executable path or its exact filesystem-canonical equivalent (for example `/var` and `/private/var`). It never falls back to basename or substring matching.

Clean-host behavioral evidence is still required before presenting generic recovery as launch-grade capability.

## SNE ownership boundary

SNE is not enrolled as a generic `checkpointed_process` recovery target. Its
runtime identity, model identity, precision, execution mode, memory admission,
and post-start readiness form one atomic launch contract that the generic
application-recovery registry cannot express.

Pantheon's SNE supervisor remains the sole lifecycle owner for SNE. It admits an
exact child identity, monitors that admitted process generation, replaces it
only after the governed readiness-failure threshold, and requires the
replacement to pass complete identity and readiness admission before service is
restored. Generic recovery may report that capability through shared product
surfaces, but it must not independently stop or launch `sned`.

This prevents two restart controllers from racing, prevents a generic relaunch
from bypassing model or memory admission, and preserves the portfolio rule of
one owner per action.
