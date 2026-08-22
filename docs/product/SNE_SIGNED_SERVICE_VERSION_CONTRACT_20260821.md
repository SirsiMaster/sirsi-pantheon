# SNE Signed Service Version Contract

Date: 2026-08-21
Status: implemented and focused-test proven

## Problem

Pantheon previously launched every packaged `sned` without `--version`. The service therefore reported its source default `0.1.0-dev` even when the executable came from a signed GA package. This weakened diagnostics, upgrade verification, clean-host evidence, and supportability.

## Contract

`RuntimePackage` now supports an explicitly signed `service_version`. For backward-compatible signed catalogs, Pantheon may derive a version only from a strict package identity beginning `SNE-X.Y.Z-`. Arbitrary labels and development strings are not accepted.

Lifecycle resolution fails closed when no canonical version exists. The dedicated SNE supervisor passes the admitted value through `--version`, alongside the exact runtime SHA-256, model manifest, checkpoint, tokenizer, metallib, MLX dylib, queue limits, and timeout.

## Evidence

- `go test ./internal/sne ./internal/dashboard`
- Unit coverage proves explicit, package-ID-derived, package-root-derived, and invalid version cases.

This changes service identity only. It is not M1 or M5 performance evidence and does not authorize architecture projection.

