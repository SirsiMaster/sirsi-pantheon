# SNE Supervised Restart Gate

Date: 2026-08-16  
Classification: platform-foundation, pre-admission

## Commercialization gate

- Buyer/user: Pantheon and Nexus operators running local Gemma 4 services.
- Pain: native MLX teardown and reconstruction is not reliably safe inside one
  process; a failed reload can make the provider disappear without a response.
- Primary workflow: request a model reload and receive the same admitted model
  from a fresh, hash-verified, ready process.
- Value: predictable recovery protects local serving availability and makes the
  engine supportable in Pantheon-managed products.
- Trust boundary: loopback HTTP, local process control, manifest-bound model,
  explicit executable/MLX/native-library paths, and no telemetry.
- Operational owner: Pantheon owns process lifecycle; SNE owns inference state.
- Done evidence: structured restart-required response, distinct child PIDs,
  readiness recovery, exact content identity, focused tests, and sealed hashes.

## Why the boundary changed

SNE previously attempted unload/load/reload on one OS-thread-owned native MLX
runtime. Two real attempts ended with empty HTTP replies, once during load and
once during reload. This is a demonstrated ownership hazard. The service now
keeps safe in-process unload/load, rejects reload with HTTP 409
`restart_required` before touching native state, and advertises restart as a
supervised lifecycle operation.

## Pantheon implementation

Pantheon's SNE client parses structured API errors and recognizes
`restart_required`. The supervisor serializes lifecycle operations, stops the
old child, launches the same admitted command and hash-locked artifacts in a
new process, waits for `/health/ready`, and then returns. `SIGHUP` on
`sirsi-sne-supervisor` exercises this complete reload contract.

## Real-device result

- Sealed `sned` SHA-256:
  `458b40521bf0125c7eb134b76e54415fa8c86f1fea1de7f63eedb6fa21d92b79`.
- Child before restart: PID `39938`.
- Child after restart: PID `40085`.
- Distinct-process proof: pass.
- Pre/post content SHA-256:
  `770fe7e402777a614956d7e38268ede50fa77b1d0d28c4a700bb474183e9cd08`.
- Post-restart content equality: pass.
- Post-restart readiness: pass.
- Focused `internal/sne` tests, including distinct-PID helper restart: pass.

The PID values are session evidence, not stable identities. No throughput,
physical bandwidth, cache, occupancy, or product-release claim is made.

## Artifact and dedupe boundary

Canonical machine evidence is under
`docs/evidence/sne-supervised-restart-gate-20260816`. Exact duplicate response
files are represented once plus a checksum ledger. Candidate, evidence, and
Owner Reading Room audits must remain at zero duplicate-content groups and zero
conflict-suffixed names.

## Decision

Admit the supervised-restart architecture for continued product integration.
Reject native in-process reload. Remaining gates include Nexus governed
workflow proof, repeated restart/cancellation durability, clean100, signed and
notarized packaging, clean-Mac reproduction, security review, and external
pilot evidence.
