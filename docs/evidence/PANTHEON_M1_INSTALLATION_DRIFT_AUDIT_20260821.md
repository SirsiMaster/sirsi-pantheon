# Pantheon M1 Installation Drift Audit

**Date:** 2026-08-21  
**Host:** M1 Pro, arm64, 16 GiB unified memory, macOS 26.6.2 (25G83)  
**Classification:** developer-state host; explicitly **not** clean-host proof

## User-Facing Installation State

- `/Applications/Pantheon.app`: absent.
- `~/Applications/Pantheon.app`: absent.
- `sirsi` on the normal shell path: absent.
- `ai.sirsi.pantheon` caretaker LaunchAgent: absent.
- A Pantheon dashboard candidate nevertheless runs from
  `~/Library/Application Support/Sirsi/Pantheon/candidates/`.

This is not an installation a normal user could discover, understand, upgrade,
or uninstall through the promised product surface.

## Durable SNE State Found

- Signed SNE binaries and package-local `libmlx.dylib`, `mlx.metallib`, native
  runtime libraries, manifests, and lifecycle tools are present.
- Multiple research runtime generations and model snapshots remain resident.
- The active direct runtime exposes exactly one model through `/v1/models`:
  `gemma-4-e2b-it-nvfp4-sne-v0`, plain decode, NVFP4 4-bit group-16,
  BF16 accumulator/cache/output, no assistant.
- The active package is the M1-specific SNE 1.1.8 E2B NVFP4 package; its runtime
  hash is `8c8674622c62d9fa137dfe6b3cb2faa2411421d8f0274be7c9fa1e083c166d5e`.
- SNE tools and runtime are Developer ID signed by team `9D382WV988`.

Performance from this M1 tuple must never be projected onto M5 and this plain
E2B NVFP4 identity must never be conflated with SNE v2 12B affine-8 MTP.

## Competing Ownership

Two SNE LaunchAgents coexist:

1. `ai.sirsi.sne.supervisor` targets packaged SNE 1.1.8 and is currently live.
2. `ai.sirsi.pantheon-sne-e2b` targets an older SNE 1.1.1 runtime tree.

The dashboard itself is separately owned by
`ai.sirsi.pantheon.dashboard`, targeting a candidate binary rather than a
canonical signed Pantheon application.

This violates the intended one-owner lifecycle model even though only one SNE
server was observed live at the audit instant.

## Signature Identity

- Active supervisor SHA-256:
  `f77afb374c95b7d1187058585245928708ee71a02320ba98748d36fde2c07ba9`
  and Developer ID signed as `ai.sirsi.sne.supervisor`.
- Candidate Pantheon CLI SHA-256:
  `eb696e24b6bab94439a1228065c311f14392a1fab216c662d3f8019ed095217d`.
- Candidate Pantheon CLI is ad-hoc signed, has no Team ID, and therefore is not
  the distribution identity.

## Contradictory Legacy Read Model

The running old Pantheon candidate returned all of these simultaneously:

- top-level `ready: true` and `service_state: ready`;
- active E2B NVFP4 catalog entry in `state: ready`;
- that same entry in `support_status: unqualified`;
- lifecycle state `stopped`;
- legacy fixed-capacity cache ceiling of 64 positions.

This response is not valid product readiness. It confirms why old candidate
state cannot be promoted merely because its server answers requests.

## Current-Source Prevention

Current Pantheon already requires exact lifecycle/runtime/model/manifest/API/
queue identity before projecting service readiness. This audit added a final
signed-support invariant:

- an active model must match a signed `release-supported` catalog entry;
- otherwise top-level readiness is withdrawn;
- service state becomes `support-mismatch`;
- Stop remains available for safe recovery;
- Nexus and the OpenAI-compatible model surface remain fail-closed.

Focused readiness, model-list, and Nexus tests pass. The complete dashboard
suite passes `go test ./internal/dashboard -count=1` (2.618 seconds).

## Required Clean-Host Follow-Up

1. Preserve this evidence, then remove competing developer LaunchAgents through
   a transactional installer/uninstaller, not manual deletion.
2. Install one signed, notarized `Pantheon.app` through the public package path.
3. Confirm one caretaker, one supervisor, one selected SNE package, and one
   governed model-store identity.
4. Verify upgrade, rollback, uninstall, reboot, and cold-boot fail-closed paths.
5. Run M1-specific functional and performance qualification without projecting
   M5 results.

## Transactional Ownership Repair Result

The new `sirsi sne ownership repair --confirm` path was admitted against the
observed M1 drift after focused rollback and receipt tests passed.

- Result: `accepted`.
- Retired label: `ai.sirsi.pantheon-sne-e2b`.
- Retired plist SHA-256:
  `107274561ef026c28bb0acccf5135e090e83296ed807c42f5f71ee0dd35b934c`.
- Recovery receipt:
  `~/Library/Application Support/Sirsi/Pantheon/recovery-receipts/20260821T235404Z-sne-ownership-repair`.
- Postcondition: one canonical label, zero legacy labels, one loaded owner.
- Canonical supervisor PID remained `43449`.
- Active model remained `gemma-4-e2b-it-nvfp4-sne-v0`, plain NVFP4-4 g16,
  manifest SHA-256
  `1886efb8ec163b0ddf7c8797bfab59204bfb03147409ffff6312922be6f84def`.

The receipt preserves byte-identical `.backup` and `.retired` copies of the
legacy plist, each matching the recorded SHA-256. This repairs ownership drift
without rewriting, restarting, or relabeling the active model/runtime tuple.

### Post-repair real completion

One bounded OpenAI-compatible 32-token request was run only as service
continuity evidence:

- HTTP 200, 1.008460 seconds wall time.
- 25 prompt tokens, 24 visible completion tokens, finish reason `stop`.
- Coherent one-sentence response to a varied natural-language prompt.
- Execution remained plain NVFP4-4 g16 with BF16 accumulator/cache/output and
  no assistant.
- Runtime SHA-256 remained
  `8c8674622c62d9fa137dfe6b3cb2faa2411421d8f0274be7c9fa1e083c166d5e`.
- Manifest SHA-256 remained
  `1886efb8ec163b0ddf7c8797bfab59204bfb03147409ffff6312922be6f84def`.
- Response telemetry reported 53.8028 visible tok/s and 71.7371 engine tok/s.

This single warm request proves continuity only. It is **not** a repeated,
isolated, thermal/power-qualified M1 performance claim and must not be compared
directly with M5, MTP, dense 12B, or short-response SNE v2 results.
