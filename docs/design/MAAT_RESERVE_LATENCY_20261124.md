# Ma'at reserve latency — findings, 2026-11-24

Answers Mercury item 20261111-100340 (task `maat-reserve-latency-dashboard-start`): where the time in a reserve goes, the 10 s outlier, and a faster path for a holder that re-reserves.

## Where the time goes

Cloud Run request latency for the 6 hours to about 2026-11-24 17:30Z, service side only. Each row is the newest 1,500 requests (the query limit), across all agents.

| Call | p50 | p90 | p99 | max |
|---|---|---|---|---|
| `MaatReserve` | 0.18 s | 0.38 s | 1.33 s | 3.38 s |
| `MaatMutate` | 0.15 s | 0.30 s | 0.98 s | 3.39 s |
| `MaatRead` | 0.04 s | 0.12 s | 0.75 s | 2.24 s |

On 2026-11-11, before the hot-row shrink (#1403), a reserve took about 0.82 s on the service.

Four `sirsi maat status` runs on the M1 took 0.79, 0.87, 0.96 and 1.70 s, against a typical 0.04 s for one service read. So most of a typical call on the M1 is spent outside the service: CLI start, the spool hand-off and the relay. The M1 was short on memory at the time. The individual client stages were not timed separately.

## The outlier

The reported 10–12 s reserve was at 2026-11-11 10:00:16Z. Cloud SQL restarted for maintenance from 09:59:31Z to 10:00:00Z, and Ma'at calls in 09:58–10:03Z took up to 47.5 s. The outlier falls in that window, which is the likely cause; that single request was not traced.

Changed since: maintenance moved to Sunday 07:00 UTC, and connection attempts fail after 5 s with a 503 instead of hanging (#1405).

## Faster path

1. One call for several machines: `maat-multi-resource-reserve`, replacing one call per machine.
2. Client hand-off: `router-spool-poll-and-relay-reuse`, the 150 ms polls and a fresh dial per forward.

## Not claimed

Ra did not measure the dashboard's Run-to-cables time against its 5 s target. Mercury's readback after #1403 was 2.2–2.6 s per start.
