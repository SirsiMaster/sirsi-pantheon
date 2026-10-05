# Stack Lab Wing Request — Pantheon Product and Control Plane

**Request ID:** `PT-WING-001`
**Requested:** 2026-09-11
**Requester:** `claude-pantheon`
**Proposed Stack Lab owners:** `codex-apollo` (engine-contract review) and `claude-io` (Stack Lab topology/recipe review)
**Repo authority:** Sirsi Pantheon

## Request

Admit a dedicated **Pantheon Product and Control Plane wing** into the Sirsi Apple Stack Lab. This wing will catalog Pantheon elements that consume SNE and other engine contracts, and will use the Stack Lab evidence-state ladder for design, implementation, qualification, and release decisions.

This is a request for a catalog wing and its recipes. It is not permission to start a model, mutate any host, install a service, sign/notarize an artifact, or publish a release.

## Wing scope

The wing owns the Pantheon-side surfaces:

1. Engine ABI and canonical route-selection authority.
2. MLX, oMLX, SNE plain, and SNE Native v2 connector adapters.
3. Session, identity, capability, streaming, cancellation, error, and receipt projection.
4. CLI, MCP, dashboard, menu-bar, TUI, and shared user-workflow parity.
5. Host-neutral worker/router control plane with dynamically admitted authority and constrained client operations. Each running Pantheon instance is a Horus direct client and Ra subclient for its current environment; no physical Mac is canonical.
6. Deterministic package inventory, copied-app checks, cask lifecycle, and release evidence composition.
7. Resource, accessibility, stability, and installed-host qualification recipes.

The wing does not own SNE inference mathematics, runtime internals, model artifacts, engine benchmarks, or SNE host qualification. It consumes those through exact receipts.

## Initial components and recipes

| ID | Stack Lab entry | First evidence question |
|---|---|---|
| `PT-ABI` | Shared Engine ABI | Do all connectors expose the same identity/capability/session/stream/receipt contract? |
| `PT-SNE-V2` | Native v2 loopback consumer | Does explicit SNE selection fail closed without fallback and preserve endpoint/profile/model identity? |
| `PT-ROUTE` | Canonical route selector | Can CLI, MCP, dashboard, menu-bar, and TUI show the same selected route and receipt? |
| `PT-WORKER` | Host-neutral worker plane | Is the current authority session-bound, with no competing client replica store? |
| `PT-QUAL` | Cross-engine qualification matrix | Are throughput/utilization numbers tied to workload, batch, KV, host, and resource receipts? |
| `PT-PACKAGE` | One-engine/Python-free package inventory | Does the copied app contain only declared payloads and exactly one canonical engine surface? |
| `PT-CASK` | Cask install/upgrade/rollback/uninstall | Do cask bytes, package bytes, and lifecycle receipts bind to the same artifact identity? |
| `PT-RELEASE` | Signing/notarization/publication | Are credentialed release gates separated from unsigned source/package evidence? |

## Required receipt shape

Every component or recipe submission must include:

- exact source commit/tree or artifact SHA-256;
- component/recipe ID and owner;
- host/profile and clean-state observation;
- command, environment, input identities, and output identities;
- measured result with workload and resource conditions;
- evidence state (`source-candidate`, `source-accepted`, `focused-verified`, `host-qualified`, `release-qualified`, `held`, or `rejected`);
- independent review binding and unresolved next gate;
- three-face sync state for repo, Reading Room, and Workspace.

Utilization figures without batch depth, KV budget, model/precision identity, host telemetry, and reproducible commands remain planning envelopes, not accepted capacity.

## SNE binding to carry into the wing

Pantheon consumes the explicit `sne`/`sne-native-v2` connector over the current host's loopback OpenAI-compatible API. The adapter must preserve selected engine, variant, model, environment profile, route, and receipt metadata. A missing, unhealthy, non-loopback, or unqualified endpoint fails visibly. No MLX, oMLX, CPU, cloud, Python, model, precision, or framework fallback is permitted. M1/M5 labels are evidence labels only; the same contract must adapt to any supported Mac. The local process is the Horus direct client and Ra subclient; remote authority is session- and receipt-bound.

SNE remains the authority for runtime implementation, model identity, readiness, inference quality, performance, and environment qualification. Pantheon may not promote those claims from adapter tests or source review.

## Initial open acceptance work

1. Resolve the Native v2 first-request 503 qualification defect with an SNE-owned receipt.
2. Produce matched SNE throughput at batch 1/8/32/128 for Max and Ultra with KV budgets.
3. Reconcile the exact runtime/model tuple for each environment seeking qualification; no M1/M5 tuple is universal.
4. Run Pantheon connector/CLI/MCP/dashboard parity under a fresh SSA admission.
5. Execute copied-package, one-engine, Python-free, cask lifecycle, resource, accessibility, and installed-host recipes separately.

## Portability boundary

The SNE lane has replaced the physical-host requirement with `ROLE-RECEIPT-001.G1`, defined at `/Users/thekryptodragon/Development/sirsi-inference/docs/engineering/stacklab/HOST_NEUTRAL_ROLE_RECEIPT_CONTRACT.md` (SHA-256 `0f248299b8b75595544d0d5deef2b7993e71bb609196db6c3e307998272b90c8`). Pantheon must bind an exact unexpired role receipt before any Mac is treated as router authority, evidence authority, execution host, or constrained client. M1/M5 remain host-profile labels and historical evidence only.

## Decision requested

Please accept `PT-WING-001` as the Pantheon wing, assign the initial Stack Lab component/recipe IDs above, and return the canonical wing index location plus the first review/qualification gate. Until that response is bound, this request is `source-candidate` and not a runtime or release authorization.
