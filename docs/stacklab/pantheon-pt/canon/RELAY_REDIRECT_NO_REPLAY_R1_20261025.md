# Relay redirected-POST no-replay correction

Classification: platform-foundation. SSA owns the bounded R1 source child of Ra1209 `643d7aaffda2a7a00e32d6baf5d46f45d7fce813`, following Main's explicit custody handoff. Ra's original branch, open review004353, credit and history remain intact. This is part of the one full136 release train, not another architecture or standalone release.

## Cause

The actual net/http TLS error-type correction closes the earlier misleading-text case, but a genuine handshake timeout can occur after an original POST has already reached the service: automatic307/308 redirects preserve its replayable body. A failure at the redirected endpoint is not proof the original service never received the mutation. The relay must not retry or durably classify that original request as never reached.

## Plan

1. Add an owned actual Client.Do regression with original POST307/308 and a would-stall HTTPS target. Require original count1, no redirected connection/authenticated request, observed redirect status and no never-reached hold. Run it against exact old643 source for a meaningful causal negative.
2. Copy the HTTP client for this forward only and refuse redirects with http.ErrUseLastResponse. Preserve shared transport/TLS pins, timeout and other client state. Use the same scoped client for the one permitted fresh retry. Never mutate the shared client's redirect policy.
3. Join the owned stalled-handshake fixture's Accept loop before closing its final connection list, eliminating the advisory cleanup race without changing production lifecycle policy.
4. Qualify all affected existing handshake/redial/no-post-send-retry and new redirect controls in normal/race under fresh CPU1 Ma'at admission. Preserve exact logs, old negative and temporary/process cleanup. Obtain independent exact-head review; Main composes into the existing full136 source and SSA remains sole release publisher.

## Commercialization Gate

User: teams sending agent work and state changes through the canonical router. Pain: an automatic redirect followed by a real handshake failure can re-send an already-received mutation. Workflow: one canonical service submission with truthful retry/unknown behavior. Value: reliable delivery without duplicated work; no separate price/license claim. Trust: existing relay token/host pin and transport unchanged; only owned loopback tests, no real token, network service, account, security configuration or deployment. Operational owner: Ra for router operations, SSA for this bounded source correction and release, Main for composition. Done evidence: actual old-source causal negative, fixed307/308 original-count/target/auth/no-hold assertions, shared-client preservation, existing safety tests normal/race, exact review, then actual integrated release/installed verification. Source tests do not prove whole-fabric delivery or full exactly-once semantics.

## Actual finite qualification

The original643 production source failed both actual307/308 cases under native4d912eb338254ac2a83fd06b85d9cee1: each delivered two authenticated original POSTs, made two redirected connections and incorrectly returned the never-reached hold. Raw `/private/tmp/ssa-router1209-r1.vT1Gg5Il/` retained.

After correction, final nativec9a5873fbce94861900eb9a3d5a9d95c passed26 named parent/subtest events in normal mode and26 in race mode, zero skips/failures, plus routerstore vet and diff checks. Selector includes all affected `NeverReachedService`, `Forward`, `RelayHTTPClient` and `RelayClient` controls, including pin enforcement. Raw `/private/tmp/ssa-router1209-r1.Z1UKeu37/`; initial green405deae6 is retained separately. The final runner records actual controller PID/PGID18155, start SatOct24 21:04:04; later exact-group process readback is empty, and owned TMP is empty. This is the owned test group, not a whole-host idle claim.

Native8a6908e393654175a25625b5eb2e7b85 repeats the causal negative using final unchanged fixture bytes with a compiler overlay of only the exact643 production `spool.go`: both redirect cases fail with two original POSTs. `/private/tmp/ssa-router1209-r1-final-control.CAoNdGWC/` retains the exact old source, overlay, source hashes, JSON events, nonzero test exit and causal assertions. The overall control job succeeds because that specific old-source failure is expected. No working source file is replaced by the control.

These are fresh CPU1 admitted, real owned loopback Go HTTP tests. No live canonical router item or production authenticated service, signer, model, credentials, host trust, service installation or release is touched. Independent source/evidence review and actual full136 composition/publication remain required; no installed no-replay or whole-fabric completion is inferred. Main must preserve the complete original Ra1209 plus this child (the full baselineb0-to-successor delta), not apply only the child while omitting the original strict TLS-type classifier and its controls.
