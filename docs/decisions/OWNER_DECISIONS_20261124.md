# Owner decisions of 2026-11-24 — record

**Recorded:** 2026-11-24 by Ra. **Source:** the owner's answers to decision cards in the Ra session of 2026-11-24. The carrying task is `ra/owner-decisions-20261124-record`.

## Decisions

| Question | Owner's answer |
|---|---|
| Cancel six stale Ra tasks? | "Fix them and actually close them if they are suspended then they are either withdrawn or retired." |
| May one merged PR close every task its description names? | Yes, if it names them. |
| Should `horus supervise` run on the M1? | Leave it running. This replaces the 2026-10-09 stop. |
| How is the exposed M1 relay credential (2026-11-23) handled? | Fix the replacement tool first, then rotate. |

## Disposition of the six tasks

Withdrawn means not done. No completion, proof or qualification is claimed by withdrawing.

| Task | Disposition by Ra under that answer | Reopen only if |
|---|---|---|
| `ra-horus-seven-fleet-successor-custody-20261013` | **Withdrawn.** The row records that terminal dispositions (SSA 232031) supersede the awaited transfer and that there are no new executable requests. The runtime supervisor, broker and CompassPoint originals it names stay open where they are. | A new executable request names one of the seven originals. |
| `ra-display-emitter-proof-20261031-ix254` | **Withdrawn, unproven.** The historical build, install and executed-hook witness is absent from the inspected records. The partial reply 20261031-144702 stands. | That witness is found. |
| `ra-stage73-qualified-scope-20261024-by88` | **Withdrawn, not qualified.** The stage 73 host is outside the managed allowed hosts and the token's domains, and no substitute is authorized. | Stage 73 is admitted by policy and a qualification is requested again. |
| `ra-check02-scheduling-custody-20261101-ji265` | **Withdrawn.** No slot was issued and the 2026-11-01 request has no recorded custodian answer. Scheduling needs a fresh request. | A new finite slot is requested. |
| `ra-m1-publisher-clearance-join-20261103-lc311` | **Withdrawn, not joined.** The Home interval it relies on has expired. SSA's signer limits are unchanged. | A current clearance is requested. |
| `router-pg-pool6-capacity-qualification-20261025` | **Decided: keep the pool at 4 per process.** SSA observed 100 max connections, 3 reserved, 97 ordinary. A pool of 6 on 20 instances would need 120. 4 on 20 instances is 80, which leaves 17 for everything else. The default in `postgresMaxConns` is already 4. | Cloud SQL `max_connections` or the instance limit changes, or the pool of 4 is measured to be a bottleneck. |
