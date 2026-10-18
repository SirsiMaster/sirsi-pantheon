# Native job admission during router transport failures

The Ma’at native supervisor keeps admitted foreground work attached to its exact
reservation and actual executor. Operators use the existing `sirsi-maat-run`
command and its existing flags; no grace-period flag or account change is needed.

If a heartbeat encounters a recognized temporary transport outage, the job can
continue only until the last proven lease deadline. The supervisor reads back
the same reservation before trying a renewal. It never creates a replacement
reservation or assumes an unanswered request succeeded. A successful confirmed
renewal can advance the deadline. A read alone cannot.

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
