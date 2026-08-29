# SNE Commercialization and Support Provenance

**Date:** 2026-08-21  
**Classification:** `platform-foundation`  
**Status:** SOURCE GATE PASSED; LIVE PILOT QUALIFICATION PENDING

## Product decision

Pantheon's SNE workflow now has an explicit repo-local commercialization profile at `docs/COMMERCIALIZATION_GATE.md`. It identifies the Mac owner/operator, the local-AI installation and recovery pain, the primary workflow, commercial value, trust boundary, operational ownership, and exact evidence required before any tuple may be described as a pilot or launchable product.

The profile preserves the portfolio boundary:

- Pantheon discovers, verifies, installs, admits, supervises, diagnoses, updates, rolls back, removes, and recovers.
- SNE computes.
- Nexus presents conversation and chooses policy-visible model intent.

## Support provenance repair

Pantheon's privacy-safe SNE diagnostics previously exposed model/runtime names and resource state but omitted exact identities needed to reproduce a packaged failure. The diagnostics now include:

- active runtime SHA-256;
- active model-manifest SHA-256;
- active resource profile;
- cache topology;
- serving cache capacity;
- prefix-session maximum and support status.

These fields are non-secret package and capability identities. The export continues to exclude conversations, prompts, generated text, model data, caches, access tokens, authorization headers, private keys, environment values, private filesystem paths, network configuration, and machine identifiers.

## Verification

The focused command passed:

```text
go test ./internal/dashboard ./internal/sne ./internal/snemodels
```

Results:

- `internal/dashboard`: pass
- `internal/sne`: pass
- `internal/snemodels`: pass

The privacy test now requires the exact runtime/manifest/profile/cache identities while retaining the full forbidden-value scan.

Draft completion proof `.agents/proofs/sne-v2-api4096-product-integration-20260821.json` also passes the structural completion validator. It remains `draft`; its blockers explicitly retain M5 qualification, M1 qualification, Workspace synchronization, and broader release evidence.

## Claim boundary

This work improves product and support closure. It does not promote the API4096 research tuple, prove a Metal run, or establish performance. The copied SNE v2 candidate still requires its complete M5 qualification receipt and later clean-host/product gates.

## Human-access synchronization

- Canonical repository source: this file.
- Owner Reading Room: mirrored in `SNE/Product` and `Pantheon` as Markdown and HTML.
- Sirsi Google Workspace: https://docs.google.com/document/d/1b-uVcZsWJ9tfjiti2ZSqiNQHCEWZdJJJtMA_efswfJY/edit (native Google Doc; created 2026-08-21).
# Exact-tuple readiness projection closure

Pantheon's dashboard and OpenAI-compatible discovery projection now fail closed unless the live SNE service matches the lifecycle tuple Pantheon actually supervised. A generic, stale, or rogue loopback process cannot become user-visible as ready merely by returning successful health endpoints. The projection requires the lifecycle to be ready and requires exact agreement on API v0, `sne.openai-chat.v2`, profile, runtime SHA-256, one model ID, model-manifest SHA-256, concurrency, queue policy, and request timeout.

Adversarial tests cover unsupervised state and runtime, model, manifest, profile, API-contract, multiple-model, queue, and timeout drift. The focused command `go test ./internal/dashboard ./internal/sne ./internal/snemodels` passes. This closes a product-integrity gap only; it does not qualify API4096 on Metal or promote a performance claim.

The recovery projection also distinguishes an absent/stopped service from a ready-looking service whose identity does not match Pantheon's supervised tuple. The latter is reported as `identity-mismatch` with an instruction to stop the unverified local service and restart the installed model through Pantheon. Generic start guidance no longer overwrites that higher-specificity recovery.

## Human-access publication

- Canonical repository source: this document.
- Owner Reading Room: Markdown and HTML mirrors under both `SNE/Product` and `Pantheon`.
- Native Sirsi Google Workspace copy: [SNE Commercialization and Support Provenance - 2026-08-21](https://docs.google.com/document/d/1kUgEvjTuG9oabxzlRCqUOAkG2QwzQoJOsJnLCm8fiSI).

Connector readback verified that the native document is non-empty and contains the exact-tuple readiness and identity-mismatch recovery sections. An earlier stale import was retained but renamed with a `SUPERSEDED` prefix; it is not canonical.
