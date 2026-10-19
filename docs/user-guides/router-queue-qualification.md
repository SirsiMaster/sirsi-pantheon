# Qualifying an operating router lane

Classification: platform foundation. Ra owns operations; SSA/Ma'at verifies
the results. An installed release, a fresh heartbeat and a `WAKEABLE` label are
not proof that a lane processed its inbox.

## One authority and one active owner

Use the installed `~/.local/bin/sirsi` and canonical service/physical relay.
Never open a retired `router.db`, copy a bearer token or adopt another session's
thread. A lane may execute on an enrolled destination using existing permitted
transport and a destination-local registered executor. Its originating host PID
is not an executor identity on a different Mac.

The owner-requested all-lane repair restores `ra.worker_answers=true` for Ra's
existing configured worker. Service-side answer-role enforcement is unchanged.
Fresh attended-session fencing still prevents a headless worker racing its lead.

Mercury has an existing busy attended lead. Its standby command is declared so
the separate wake loop does not claim there is no consumer invocation. The
permanent `attended=true` fence is retained, with no worker-answer grant, so this
declaration cannot start a competing Mercury worker. Only its lead's own queue
action qualifies current Mercury consumption. Do not replace a WATCH_ONLY label
with a fabricated success, publish a heartbeat on the lead's behalf, or resume
and copy the busy Claude session. Any future standby activation needs the lead's
explicit handback and answering grant; changing the declaration is not handback.

## Observe what is actually loaded

On each Mac, compare the installed signed CLI with each existing wake process's
loaded executable, not only the path in its launchd plist. A process can still
run the old inode after a correct on-disk upgrade. Restart only an exact identified
controller through its supported lifecycle path. For an active detached consumer,
first verify the durable consumer marker, PID/start identity and separate process
group; verify that the replacement controller adopts the same surviving consumer.
Never pattern-kill a lane, stop its payload, clear a quarantine without diagnosis,
or reset a foreign task lease to make the board look green.

## Select transport explicitly

Scoped worker runtimes keep the existing default:

```sh
sirsi-fabric-run --plan m1 --work own-proof --cpu-cores 1 --timeout 90 -- /usr/bin/true
```

An already-authorized attended runtime with no configured network proxy may
explicitly select the existing pinned direct SSH path:

```sh
sirsi-fabric-run --plan --transport direct studio --work own-proof --cpu-cores 1 --timeout 90 -- /usr/bin/true
```

`--plan` never executes the payload. Remove it only for actual admitted work.
Direct mode does not grant network access, SSH authentication, destination
permissions or an admission exemption. If the runtime denies the connection,
report the denial; do not route around it. A configured proxy makes direct mode
fail before either supervisor or SSH is launched. Default proxy mode still fails
closed when the scoped SOCKS endpoint is missing. There is no fallback or retry
to a different transport. M1, M5 and Studio use their enrolled stable Tailscale
addresses, and retain existing strict key aliases and known_hosts files.

## Acceptance record

For each working lane, retain the exact request ID, recipient acknowledgement,
actual recipient thread/runtime and host, installed CLI hash, returned result,
and independent canonical close/response readback. For destination execution,
also retain the actual destination executor, granted admission, command result,
released reservation, closed executor and residual cleanup debt. A harmless
`true` execution proves placement, not application tests or backlog completion.

Historical aliases, human mailboxes and deterministic services need an explicit
disposition; they must not be counted as failed LLM workers or silently deleted.
Never mark the whole fabric complete while one active lane lacks its result.
