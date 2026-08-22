# SNE Graph-Caretaker Crash Memory Admission Repair

Date: 2026-08-20

Classification: `platform-foundation`

## Commercialization Gate

- **Buyer/User:** A Mac owner running local SNE inference through Pantheon while continuing ordinary foreground work.
- **Pain:** A supervised crash restart can be output-correct yet displace live application pages into swap, making the Mac feel broken and invalidating clean performance evidence.
- **Primary Workflow:** Pantheon admits, supervises, crashes or restarts, and recovers the exact SNE package without silently exhausting unified-memory headroom.
- **Willingness To Pay:** Reliable local inference and machine care are core trust, retention, and premium Nexus/Pantheon operating value.
- **Trust Boundary:** Local process lifecycle and aggregate resource measurements only; no prompt content, credentials, private paths, or cloud fallback.
- **Operational Owner:** Pantheon owns admission, lifecycle, memory/swap care, recovery, and operator diagnostics. SNE owns truthful measured runtime requirements.
- **Done Evidence:** Focused Go tests, exact package identities, bounded real crash evidence, low-swap rerun after the copied SNE manifest correction, and owner-visible recovery guidance.

## Incident

The copied **Shared-Wide MTP 131 Practical Context 4K Graph-Caretaker** passed an eight-hour clean-session soak, then passed the correctness portion of a bounded crash/restart gate. SIGKILL produced exit 137, the endpoint closed, a distinct process restarted, no prefix session survived, both recovered outputs exactly matched the stateless control, and readiness returned.

Resource admission failed. Swap grew from 2.06 MiB to 3,259.81 MiB. The restarted service reported 24,014,487,552 RSS bytes. The overnight Graph-Caretaker run had already observed 25,545,459,702 active MLX bytes, while the package manifest declared only 21,474,836,480 bytes.

The old process was reaped before the replacement started, so this was not two SNE services overlapping. The replacement model load displaced foreground pages because Pantheon trusted an understated manifest and real lifecycle tests could omit `RequiredMemoryBytes`, bypassing admission.

## Repair

Pantheon now:

1. Reads the launch manifest inside `NewSupervisor`.
2. Verifies its exact SHA-256 against the admitted launch identity.
3. Derives required memory from the manifest when callers omit it.
4. Rejects caller/manifest memory disagreement.
5. Rejects manifests with no measured footprint.
6. Adds a node-relative lifecycle reserve of total RAM / 12 for crash, reload, supervised restart, and load-after-unload paths.
7. Preserves the existing live available-RAM, dynamic reserve, memory-pressure, death-spiral, and swap-limit checks.
8. Prints `lifecycle_reserve_bytes` separately in supervisor operator output and real lifecycle evidence, so restart headroom is visible rather than folded into an unexplained denial.

Focused `go test ./internal/sne` passes. Test fixtures now use real temporary manifest bytes and hashes and inject an explicit safe resource sample instead of accidentally consulting the live host.

Focused `go test ./cmd/sirsi-sne-supervisor ./internal/sne` also passes after the operator-output addition.

## SNE correction

The admitted Graph-Caretaker candidate remains immutable. A copied descendant was created:

`artifacts/candidates/shared-wide-mtp-131-practical-context-4k-graph-caretaker-measured-memory-v1-20260820-candidate`

- Human name: **Shared-Wide MTP 131 Practical Context 4K Graph-Caretaker Measured-Memory**
- Corrected required memory: 25,545,459,702 bytes
- Model-manifest SHA-256: `b90829d0793d7aaf96836dc47a5b4264d78ce944c91e42ac6a346f44f4a2cc6e`
- Service, native runtime, MLX dylib, metallib, model, assistant, arithmetic, and source closure are unchanged.
- Status: constructed, not executed, because current swap exceeds Pantheon admission policy.

## Claim boundary

Crash correctness passes for the parent Graph-Caretaker package. Crash memory recovery does not. The measured-memory descendant has no performance or lifecycle claim until a low-swap identity, exactness, and bounded restart gate passes. No physical bandwidth, occupancy, SLC, cache, MMU, register, or ANE claim follows from this work.

## Prevention rule

No SNE process may launch with an absent, caller-invented, or manifest-disagreeing memory footprint. Normal launch and lifecycle restart are distinct admission events. A lifecycle test that proves output recovery while materially growing swap is not an admitted recovery test.

## Actionable recovery contract

Pantheon now returns typed, privacy-safe admission failures to the CLI, UI, Nexus, and support surfaces. Each denial carries a stable code, a human-readable explanation, and an actionable recovery step without exposing process arguments, paths, prompts, credentials, or model data. Current codes cover missing measured footprints, unavailable RAM/swap measurements, memory emergency or pressure, excessive swap, and insufficient lifecycle headroom.

The supervisor records the complete failed admission sample before returning the error. Operator output therefore includes the real required bytes, available bytes, dynamic reserve, lifecycle reserve, swap use and limit, and pressure state rather than zero-valued diagnostics. Failed lifecycle admission retains the parent supervisor context so a normal user can free memory and retry without reconstructing Pantheon.

Focused verification passed: `go test ./cmd/sirsi-sne-supervisor ./internal/sne`.

This repair is code-qualified only. It does not admit the measured-memory SNE descendant; that package still requires a clean low-swap runtime gate.

## Pantheon product surface

The SNE lifecycle API now exposes `error_code`, `recovery`, and the measured `resource_admission` object for typed admission denials. The Pantheon SNE screen renders required versus available memory, dynamic and lifecycle reserves, swap use versus limit, the prescribed recovery action, and an accessible **Retry when conditions are safe** control bound to the same model/runtime identity. It never retries automatically.

Privacy-safe support diagnostics preserve the failed admission snapshot rather than replacing it with a later host sample. Free-form internal error text remains excluded from the export. `go test ./internal/dashboard` passes for lifecycle propagation, UI compilation, support-diagnostic identity, and sensitive-field exclusion.

## Stable OpenAI-compatible error contract

Pantheon's `/v1/models` and `/v1/chat/completions` routes now return an OpenAI-compatible error object with `message`, `type`, `param`, and stable `code`. A privacy-safe `sne` extension declares `no_fallback: true` and may include the governed recovery action plus measured required memory, available memory, swap use, and swap limit. Low-level dial errors and arbitrary upstream response bodies are never returned to the caller.

The focused dashboard suite proves model-identity mismatch and swap-recovery envelopes. Successful SSE payloads remain byte-preserved.
