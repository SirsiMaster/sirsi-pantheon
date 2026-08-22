# SNE v26 Signed Pantheon-to-Nexus Product Bridge

Date: 2026-08-18  
Status: local product integration admitted; broad launch and public performance qualification remain open

## What passed

Pantheon installed a new signed `sne-gemma4-v2` runtime catalog without changing
the signed `v1` predecessor. The catalog admits the durable SNE v26 package as
the sole active 12B affine-8 plain execution tuple while retaining the older
package in the runtime catalog for rollback.

- catalog SHA-256: `aca14182c98fdab493491322d14d49ed84a6da861b718a03164bffe513101019`
- prior catalog SHA-256: `bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d`
- model: `gemma-4-12b-it-affine8-sne-v26-capacity-readiness-candidate`
- runtime/package: `SNE-2.5.0-12b-affine8-paged-v26-candidate-v1-darwin-arm64`
- service SHA-256: `fc979e2031fc2459b6ecca96d3d6d85cafa8d898d7e3bcda90bda1e98f2c0787`
- native runtime SHA-256: `ef6a2a4b3fffee88522f60de30e27690d8881dac6bcfb70fbe09421dcd905f65`
- cache contract: paged ring, 4096 positions, two bounded prefix sessions

Pantheon reported the exact signed runtime identity during `starting` and
`ready`. A real Nexus-shaped request then returned HTTP 200 with 34 SSE events,
one terminal `[DONE]`, first response bytes at 0.210 seconds, and total time
1.088 seconds. The exact local model produced a coherent two-sentence answer.
No Python, CPU, cloud, model, precision, or framework fallback was used.

## Product corrections

Nexus no longer trusts the stale router-board file for inference endpoint or
model identity. Pantheon's signed live read model is the sole authority for
catalog, lifecycle, model, runtime, readiness, and service URL. The input stays
disabled if any identity component is absent or disagrees.

The v26 package and model checkout were installed with APFS clone semantics so
the durable product paths do not require another physical 13 GB copy. The new
checkout has its own exact receipt and remains independently verifiable.

## Failures converted into gates

1. The old 48-position package correctly rejected streaming. Product UI may not
   route to a package whose advertised contract cannot satisfy the UI.
2. A signed predecessor was briefly edited before signing. The edit was removed,
   v1 hashes were restored, and v2 became a new append-only lineage member.
3. Pantheon rejected duplicate active execution tuples. v26 is now the one active
   8-bit plain tuple; the old package remains rollback material only.
4. The evidence parser incorrectly passed `[DONE]` to `jq`. Protocol markers are
   now separated from JSON payloads before semantic validation.
5. Host-local API probes from the restricted sandbox can report false connection
   failures. Real localhost product probes must execute in the host boundary.

Executable prevention is in `scripts/check-sne-signed-catalog-lineage.zsh`.

## Claim boundary

This proves a real, useful local conversation through the signed v26 package.
It does not yet establish a public tok/s claim, sustained generation speed,
quality across varied prompts, long-context retrieval, concurrency fairness,
fresh100 durability, soak behavior, M1 support, or four-arm superiority. Those
remain mandatory phases of the active SNE v2 launch goal.
