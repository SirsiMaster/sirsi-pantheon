# Mercury read-only consumer

`Reader` is the shared read path for CLI and MCP. It consumes Mercury's
versioned dashboard v1 snapshot and projects one `Read` DTO; consumers should
render that DTO rather than maintain separate connection, freshness, or metric
state. This package never starts, stops, or configures a Mercury run.

Configuration comes from `SIRSI_MERCURY_DASHBOARD_URL`. If the variable is
unset, `NewConfigured` uses the fixed local endpoint
`http://127.0.0.1:8099/api/v1/dashboard`. If it is explicitly empty, the reader
is unconfigured and makes no request. Explicit endpoints must use the dashboard
v1 path and pass the same private-address policy. There is no discovery or
public endpoint fallback. The native app's separate port-1190 protocol is not
changed by this Go client.

The reader performs GET only, rejects redirects, disables environment proxies
and keep-alives, revalidates every resolved address at dial time, and bounds a
read to two seconds and 262144 response bytes. Context cancellation propagates.
Errors do not echo endpoint strings, response bodies, or transport details.

Contract types and the embedded schema are derived from Mercury's
`mercury.dashboard/v1` producer contract. Schema version 1 minor additions and
future enum words remain compatible; required structure, numeric ranges,
calibration consistency, and link identity are validated. Rates are decimal
GB/s. Unknown measurements stay null and never become zero. Stale reads retain
the observed snapshot but suppress live totals and motion.

`Headline` and `Scene` are shared presentation projections. CLI/MCP consumers
should use their provided values and labels; they must not independently infer
units, gain, topology, hardware family, or freshness. `Client.HTTP` and `Client.Now`
are injectable for deterministic tests; production clients use the bounded
private transport above.
