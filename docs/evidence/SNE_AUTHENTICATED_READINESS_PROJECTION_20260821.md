# SNE Authenticated Readiness Projection

**Date:** 2026-08-21  
**Classification:** Commercial-product integration repair  
**Status:** Source and focused integration gates passed; live secured API4096 proof pending

## Result

Pantheon's SNE readiness projection now authenticates to the secured SNE service with the same current local capability held by Pantheon's authoritative control engine.

The prior path created an unauthenticated `sne.Client`. Secured SNE leaves health endpoints loopback-readable but requires the Pantheon-managed bearer capability for `/v1` identity, model, status, and inference routes. As a result, a healthy secured runtime could fail Pantheon's exact-tuple projection after the health check and appear unavailable to Pantheon and Nexus.

`sneReadModel` now constructs its client through `newSNEReadClient`, which snapshots the current rotation-safe capability immediately before use and passes it to `sne.NewAuthenticatedClient`.

## Rotation prevention

The regression test starts a secured fake SNE service that rejects every request without the expected bearer. It initializes Pantheon with an older capability, rotates the in-memory authority, constructs the readiness client, and proves all readiness/status/model calls use the rotated value. This prevents a second class of failure in which startup works but later capability rotation makes status permanently stale.

## Evidence

- Repair: `internal/dashboard/sne.go`
- Regression: `internal/dashboard/sne_authenticated_projection_test.go`
- Command: `go test ./internal/dashboard ./internal/sne -count=1`
- Result: both packages passed.

## Authority and claim boundary

Pantheon remains the only lifecycle and admission authority. Swift, Nexus, and other surfaces consume Pantheon's projection; none receives a second credential store or reimplements admission.

This is source/integration evidence. It does not prove a live API4096 process, model correctness, Metal availability, token throughput, clean-host packaging, or performance. Those gates remain fail-closed while the M5 graphical console is locked.

## Human-access homes

- Canonical repository source: this file.
- Owner Reading Room: `Pantheon/SNE_AUTHENTICATED_READINESS_PROJECTION_20260821.md` and `.html`.
- Native Google Workspace record: https://docs.google.com/document/d/1ptn-zI6jRXz1Y4EN9RPJZpeHrv-moK3TyGP0LiINcaU/edit
