# SNE Queue Transparency to Nexus

**Date:** 2026-08-21  
**Classification:** Commercial-product operational integration  
**Status:** Source and focused cross-repository gates passed; live contention proof pending

## Result

Pantheon now projects SNE's authenticated scheduling state to Nexus without transferring scheduler ownership.

- SNE remains the sole FIFO admission controller.
- Pantheon authenticates to `/v1/sne/metrics`, verifies the live policy matches the already admitted readiness identity, and projects active and waiting counts.
- Nexus displays the queue discipline, active/capacity count, waiting/capacity count, and request deadline.
- Missing or policy-mismatched metrics are labeled unavailable rather than rendered as zero.

## Security and identity boundary

The metrics request uses Pantheon's current rotation-safe local capability. Dynamic telemetry is admitted only when maximum concurrency, maximum queue depth, discipline, and timeout exactly match the readiness identity. Nexus receives no SNE credential authority and performs no scheduling decision.

## Evidence

- Pantheon client: `internal/sne/client.go`
- Pantheon projection: `internal/dashboard/sne.go`
- Pantheon regression: `internal/dashboard/sne_authenticated_projection_test.go`
- Nexus surface: `packages/sirsi-portal-app/src/components/AskSirsiPanel.tsx`
- Nexus regression: `packages/sirsi-portal-app/src/__tests__/components/AskSirsiPanel.test.tsx`
- Pantheon: `go test ./internal/dashboard ./internal/sne -count=1` passed.
- Nexus: focused SSE and Ask Sirsi suites passed 2 files / 16 tests.

The first Nexus rerun failed because an older test used an unqualified `getByRole('status')` after the screen gained a second legitimate live region. Production behavior was correct. The queue region now has an explicit accessible name and the test addresses both status regions unambiguously.

## Claim boundary

This proves source projection and deterministic UI behavior. It does not prove live FIFO ordering, queue latency, cancellation under contention, model execution, or serving performance. Those remain in the secured API4096 concurrency gate.

## Human-access homes

- Canonical repository source: this file.
- Owner Reading Room: `Pantheon/SNE_QUEUE_TRANSPARENCY_TO_NEXUS_20260821.md` and `.html`.
- Native Google Workspace record: https://docs.google.com/document/d/1BoXRSmb_fMROgd6lmNXP-230Pbu2h6VhruWPeBJFfZM/edit
