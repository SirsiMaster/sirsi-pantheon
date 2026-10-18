# Native job admission during router transport failures

The Ma’at native supervisor keeps admitted foreground work attached to its exact
reservation and actual executor. Operators use the existing `sirsi-maat-run`
command and its existing flags; no grace-period flag or account change is needed.

If a heartbeat encounters a recognized temporary transport outage, the job can
continue only until the last proven lease deadline. The supervisor reads back
the same reservation before trying a renewal. It never creates a replacement
reservation or assumes an unanswered request succeeded. A successful confirmed
renewal can advance the deadline. A read alone cannot.

Healthy remote checks run no more often than once per30 seconds, shortened
for remaining proven lease time; local child checks stay within two seconds.
A revocation is observed at the next remote check, up to30 seconds plus bounded
RPC latency and always within the proven deadline. Recovery readback retries
are sooner (1/2/4/8 seconds), and terminal acceptance always checks the exact ID.

Explicit denial, revoked/expired state, changed holder/work/executor/share, and
unrecognized failures stop the job immediately. An unresolved outage stops and
reaps the launched process group when proven time runs out. The terminal receipt
under `~/.sirsi/evidence/maat-runs/` records transport retry counts, the most recent
transport error, final status, and any cleanup debt. Do not start a replacement
job until its prior group and reservation cleanup are confirmed. A completed
child is never restarted for a recovery check.

Borrowed reservations are not renewed or released by this supervisor and remain
bounded to their initial proven interval. This is cooperative same-user job
supervision, not a security sandbox or a promise of outage-free execution.

Source fixtures: `python3 -m unittest discover -s scripts/maat -p test_admitted_run.py -v`.
SSA owns independent review, installation and long-running workload qualification.

## Executor versus IDE session identity

A destination executor has its own thread, not the launching IDE conversation's
provider session. The supervisor removes inherited Claude/Codex provider-session
IDs from executor CLI **control subprocesses only**. Registration cannot publish
a child thread into the parent's session marker, and close cannot remove that
parent marker. Control calls after registration carry the exact executor thread;
the payload's provider environment is not changed by this isolation.

For desktop task claim/renew/release, keep the same actual `SIRSI_AGENT_ID` and
`SIRSI_THREAD_ID` on every call. A task command's `--thread` argument records the
requested task worker; it is not a substitute for the authenticated caller's
thread environment. An old claim authenticated without a thread does not become
owned by a later desktop or executor thread. Preserve the ownership refusal:
never copy session credentials, borrow a worker identity, reset attempts, or
weaken the fence to recover it. Use normal owner-session release or ordinary
expiry/reclaim, then claim under the actual stable desktop identity.
