# Release consumer preservation

Component: Ra release lifecycle, under existing ADR-061 durable-delivery and native controller image-adoption controls.

An installation updates pathnames; it does not prove an already running controller loaded those bytes. Conversely, forcibly restarting all controllers can interrupt their active consumers. `scripts/release-train.sh` must neither signal every wake loop nor override Ma'at windows to publish. Existing controllers observe the installed image and yield at their safe consumer boundary. A lifecycle repair for a genuinely stuck idle controller requires its exact native generation and owned-controller evidence; this recipe is not a blanket restart authorization.

After installation, independently read kernel image identity and generation, actual consumer continuity, canonical queue results and cleanup on each enrolled host, including Studio where applicable. Busy consumers remain preserved and the rollout stays honestly incomplete until adoption is proven. The release script reports installation only, not a full fleet acceptance.

The source guard rejects the former forced-restart and automatic-override patterns and is run in hosted CI. It does not start services, install a package or simulate a successful live adoption. Existing M1/M5/Studio actual official121 receipts remain separate evidence.

Canonical repository source is authority. Owner Reading Room and native Workspace copies for this unreleased correction remain pending; do not claim three-home publication completed.

## Relay discovery and failed-test cleanup

Protected CI39374377531 hit the pre-existing relay backlog assertion and then hung in httptest.Close for ten minutes: its held handler was released only on the successful assertion path. Cleanup now cancels and frees handlers before closing the test server, including failed assertions. No failure or assertion is suppressed.

A deterministic per-instance discovery probe reproduced the underlying wake-storm branch:3,070,821 scans in460ms with the old conditional sleep (native terminal31e5a5e453f4499ba1a694c48716f422). A worker can consume its wake before its forward goroutine claims the file, so delivered wake alone cannot justify an immediate rescan. Every discovery pass now waits the existing spoolPoll interval, cancellably. Independent lane workers continue forwarding without waiting on other lanes. The existing scan-budget assertion uses actual monotonic elapsed time with the same two-pass allowance instead of assuming Sleep600ms can never overshoot under a loaded race runner. The300ms cancellation and fast-lane response requirements remain unchanged.

The probe is unexported and per relay instance; no lane/RPC/CLI option can inject it. Production uses the same real discovery implementation. Separate native race and protected PostgreSQL proofs are still required before a release claim.
