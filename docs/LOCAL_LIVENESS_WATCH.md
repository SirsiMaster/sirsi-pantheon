# Local liveness watch

The supported per-user installer selects and verifies the Sirsi executable
through the existing canonical packaged-engine contract, uses the user's HOME
as its working directory, and installs `liveness-watch run --local`.
It retains `ai.sirsi.liveness-watch`, RunAtLoad and the15-minute interval; an
idle loaded interval job need not have a persistent PID. Installation is an
explicit action, not a side effect of opening Pantheon. Operator-disabled
intent is preserved, not silently enabled. No root elevation is used.

The local run has a12-second cooperative collection deadline and2-second
native-probe deadlines. It records a deterministic System One screen in the
existing `~/.sirsi/maat/decisions.jsonl`; `sirsi maat casebook` reads that same
history. Unknown or failed probes cannot become a passing screen. Append
refusal is an error, not a recorded success. Journal I/O uses the existing
nonblocking mutation lock and synchronous append/fsync; no abandoned timeout
writer is created. OS filesystem stalls are not claimed to have a hard realtime
deadline.

The scope is local metadata: exact watch configuration and loaded state,
managed disabled overrides, optional menubar job metadata and VM free-page
metadata. A missing optional model or menubar is not an install requirement.
Loaded job metadata does not prove UI responsiveness, and readable VM counters
do not prove the absence of memory pressure. No inference, generation probe,
network request, download, process signal, automatic repair or registry write
is performed. Local host/requester metadata is not a fabricated Ra identity.

Local recording is not canonical delivery, remote acknowledgement, reservation
authority or completed work. This mode reports `ra_delivery=not_attempted`.
Existing explicitly configured legacy `liveness-watch run` remains unchanged,
including its distinct generation and router-dispatch behavior. It is not
invoked by the new local schedule. Optional Ra delivery must retain its existing
actor/declaration/session and service/outbox contracts; no unavailable route
creates a fallback router database or a registry declaration.

Ma'at's closed `liveness-watch` repair binds the canonical running engine and
the ordinary user's HOME, then uses the same `InstallLocal` and
`InspectLocalWatch` APIs as the direct local command. Its manifest hashes the
local-mode plist; its exact review, confirmation, preflight, postcondition and
Casebook receipt remain in the existing Ma'at workflow. The bounded inspector
can identify a known unloaded or legacy local schedule for review, but refuses
operator-disabled or unknown launchd state. A passing preflight is rechecked
against the complete failure-memory snapshot under the attempt-admission lock.
Historical outcome projection remains distinct from the later current-state
observation. The `status` command also reports verified only when the shared
inspector confirms the exact local configuration and loaded state—not merely
when a plist file exists. The explicit legacy `liveness-watch run` route
remains separate and unchanged.

This integration is an unqualified source candidate. It does not establish
installed repair behavior, broaden the Ma'at repair registry, or alter other
operational rows. No host installation is performed by its fixtures.
