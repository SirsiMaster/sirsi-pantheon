# Router host-pressure probe: typed-unknown caching policy

Status: source-publication decision for PR #1060 (`internal/router/backpressure.go`
R7/G6 host-pressure gate). Independent review required before any merge or
native rollout. Classification: platform-foundation. No ADR governs this
mechanism today (verified: no ADR in `docs/` or in `backpressure.go`'s own
history ties to it) — a new ADR, if the custodian decides one is warranted,
is a separate decision from this document.

## Decision

Cache a failed probe as typed "unknown" for the same 30s TTL (`hostLoadCacheTTL`)
as a successful one. The prior implementation never published a failure and
retried on every call; the prior test required `unknown -> ok=false` but did
not bound retry frequency. This change intentionally lowers retry frequency
for an unavailable sampler.

An unknown result never becomes a successful pressure reading: a cached
unknown record returns `(0, false)`, and `shouldDeferDispatch` treats unknown
as "do not defer" (fail-open) exactly as before. The cache record's `fresh`
flag is freshness of an *observation status* (known or unknown), not
evidence of CPU load.

## Why

Nine wake loops on one host asking the same question every cycle must not
turn an unavailable sampler into nine overlapping probes per cycle — that
amplification is itself a source of host load. One probe per host per TTL,
shared via `flock`, serves all of them regardless of whether the probe
succeeds or fails.

## Accepted tradeoffs and limitations (explicit, not implied)

- **Recovery latency**: a transition from unknown back to known can be
  delayed by up to the remaining TTL (30s) after the sampler becomes
  available again. Admission stays fail-open during that interval, so this
  delays *detecting* pressure, never blocks dispatch.
- **Cache-write failure permits sequential retries, not a fixed cost**: if
  persisting the result (the atomic temp-file-plus-rename) fails — e.g. a
  read-only filesystem, a renamed-away cache path — `coordinatedHostLoad`
  returns the genuine probe result for that call, but makes no
  one-probe-per-TTL guarantee afterward; every subsequent caller probes
  again until a write succeeds. There is no bounded-cost claim under
  persistent filesystem failure.
- **Mixed-version non-coordination**: only cooperating processes running
  this v2-cache-aware code participate in the shared lock/cache protocol.
  An old binary still running the prior `top`-based sampler does not read or
  write this cache and does not coordinate with upgraded processes — a
  fleet with mixed binary versions gets no cross-process sharing benefit
  until every participant is upgraded.
- **Nominal, not guaranteed, sample interval**: `iostat -w 1` requests a
  nominal one-second interval between its two aggregate rows; the OS can
  return early (e.g. on an IOKit notification waking the CFRunLoop). No
  fixed numeric temporal-error bound is established or claimed — this is a
  recent-aggregate observation, not a controlled wall-clock interval. Native
  custodian acceptance of this semantics is a separate, outstanding gate
  before any production rollout.
- **No native cost claim**: the focused and full `internal/router` test runs
  in PR #1060 measure test wall-time on one M1 development host, not
  sampler CPU/latency cost on any target production host. That measurement
  is explicitly out of scope for this source-publication slice.

## What is preserved unchanged

`backpressure_test.go` (existing dispatch-admission behavior: unknown never
holds dispatch, known high/low load gates correctly) is byte-for-byte
unchanged. The migrated `backpressure_cache_test.go` uses real on-disk
expired cache records (no global mutable clock seam), and still verifies
both directions (low-to-high, high-to-low), known-to-unknown and
unknown-to-known TTL recovery, and rejection of corrupt/legacy/future/stale
cache records plus non-finite/negative normalization. Cross-process tests
(`coordination_test.go`) cover cold/stale/unknown/crash locking and
publication/lock-acquisition-failure paths, including the cold-home case
(a missing `~/.sirsi` parent directory, PR #1060 CI finding) which this
change now creates rather than treating as a failure.

## Outstanding, unresolved by this document

Native recent-variable `iostat` semantics acceptance, golden-output
validation on a controlled target host, aggregate sampler cost
qualification, delivery continuity across a mixed-version fleet rollout,
and any merge/native-install/service-restart authorization all remain
separate, outstanding gates. This document does not close any of them.
