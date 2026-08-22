# SNE OpenAI Cryptographic Response Identity

Date: 2026-08-21
Status: implemented and focused-test proven

## Contract

Pantheon is the sole SNE lifecycle and admission authority. A model is exposed through `GET /v1/models` only when the signed catalog, active model, runtime ID, runtime binary SHA-256, model-manifest SHA-256, and serving profile are all present and admitted. Both hashes must be canonical lowercase 64-character SHA-256 values.

The lifecycle resolver computes these hashes from the package boundary before launch. The SNE supervisor independently proves the same values through exact readiness admission. Pantheon now carries that verified identity into its lifecycle snapshot and OpenAI-compatible model response.

The completion boundary must return the same model, runtime SHA-256, model-manifest SHA-256, and profile. Consumers must reject a response that omits or changes any member of that tuple. A late streaming mismatch may preserve already displayed partial output for the user, but the request is not a successful governed completion and no alternate runtime, model, cloud, CPU, Python, or framework fallback is permitted.

## Ownership Boundary

Pantheon's dedicated SNE supervisor remains the only process restart controller for `sned`. Generic application recovery must not launch or replace it independently because doing so would bypass atomic model/runtime/precision/memory re-admission.

## Evidence

- `go test ./internal/dashboard ./internal/sne`
- Pantheon API tests reject malformed and uppercase SHA-256 identities.
- Nexus Go and Ask Sirsi tests prove exact acceptance and terminal mismatch rejection.

