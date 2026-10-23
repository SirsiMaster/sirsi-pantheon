# Mercury monitor in Pantheon

`sirsi mercury` reads the existing Mercury dashboard and presents its current
state. `sirsi mercury --json` returns the complete structured read, including
source timestamps, nodes, links, calibration, unknown values and run state.
It does not start a run, probe bandwidth, sample a device or change a service.

The MCP tool `mercury_status` returns the same Go read model. It takes no
arguments: a model cannot turn it into an arbitrary URL fetcher or a run-control
tool. Mercury remains the authority for measurements; Pantheon is a consumer.

## Connection and recovery

By default, the reader uses the existing local monitor at
`http://127.0.0.1:8099/api/v1/dashboard`. An operator can select an existing
private monitor with `SIRSI_MERCURY_DASHBOARD_URL`. An explicitly empty value
disables reading and reports unconfigured. There is no automatic discovery,
public-network destination, proxy use, redirect, authentication change or
monitor installation.

Each read is bounded to two seconds, 8192 response-header bytes and 262144
response-body bytes. A failed read
reports unavailable, refused permission or incompatible data rather than
inventing a live snapshot. Refresh means invoking the read again; no background
cache is promoted to live state. If the monitor is unconfigured, select the
existing approved monitor. If it is unavailable, inspect its owned service and
Ra connectivity through their normal controls; this read-only command does not
silently restart it. Unknown measurements are null, not zero.

The snapshot retains the producer's original observations as evidence. Use the
assessed headline, scene and totals for current claims, not a raw historical
number inside that snapshot. Eligibility is evaluated at response completion;
expired or excessively future observations cannot animate traffic or contribute
to a current aggregate. Unavailable measurements are not labelled idle, and an
incomplete aggregate is not presented as a complete total.
`read_at` records when the request started; it is not a measurement timestamp.
Producer `generated_at` and observation timestamps remain the measurement
evidence, and freshness is assessed once after the response is decoded.

Wire rates use the producer's decimal units. Compressed payload, gain,
calibration and run status retain their producer meaning. A successful GET does
not establish throughput qualification, quiet-window acceptance or all-machine
availability. The state and source timestamps must be interpreted together.

## Native app and web scope

The current native Mercury workspace reads the existing `/api` hardware monitor
for CPU, GPU residency, power and interfaces. That is a different contract from
the versioned `/api/v1/dashboard` read used here. This increment does not replace
that UI, change its controls or claim byte-for-byte native parity.

The recovered historical web/SVG and native scene designs are explicitly
accounted for in `docs/stacklab/MERCURY_CONSUMER_ADOPTION_20261023.md`. They are
not silently imported, deleted or represented as shipped by this CLI/MCP work.

## Method and release

Stack Lab recipe: `contracts/stacklab/mercury-consumer-recipe-v1.json`.
The shared core is `internal/mercury`; CLI and MCP remain thin adapters. Its
fixture suite covers producer samples, unknown fields, units, stale/future
snapshots, private address boundaries, malformed responses and cancellation.
The package inventory admits the exact recipe name, not an arbitrary folder.

Classification: platform-foundation until the integrated signed successor is
qualified. The operator and their local agents gain the same truthful monitor
view without a paid model. No independent willingness-to-pay validation is
claimed. Mercury owns producer operations, Pantheon owns consumer behavior, and
SSA owns signed release/installation. Completion evidence for this increment is
source review, focused normal/race/refusal tests, CLI/MCP parity and a bounded
live CLI readback; those do not substitute for installed-product evidence.

Repository source is authoritative. Reading Room and Workspace mirrors await
SSA's integrated runbook publication and are not claimed synchronized here.
