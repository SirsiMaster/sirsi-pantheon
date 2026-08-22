# SNE v26 Real Browser Product Proof

Date: 2026-08-18

## Result

The Nexus Agent Fleet page completed a real local conversation through the
Pantheon-governed SNE v26 runtime. The page visibly reported:

- device: Apple M5 Max
- state: ready
- model: `gemma-4-12b-it-affine8-sne-v26-capacity-readiness-candidate`
- runtime: `SNE-2.5.0-12b-affine8-paged-v26-candidate-v1-darwin-arm64`
- signed catalog digest prefix: `aca14182c98f`
- route: Pantheon-managed local SNE

The prompt asked for three concise sentences explaining rainbow formation. The
UI streamed and rendered a coherent three-sentence response and did not use a
cloud, Python, alternate-model, or framework fallback.

This is a product usability and governance proof. It is not a new throughput,
power, thermal, long-context, or release-performance claim.

## Defects caught and repaired by the real browser gate

1. Pantheon allowed Vite port 5183 but omitted the actual default port 5173.
2. The live ledger response omitted arrays that `LedgerBoard` assumed existed.
3. Nexus connected directly to SNE port 8477, creating a second browser CORS and
   trust boundary.
4. The first proxy URL used a duplicated `/api/sne` path because the loopback
   guard composes by exact string concatenation.
5. The stream test matched `/api/sne` before `/api/sne/chat`, masking the child
   route with a broad parent prefix.

## Permanent controls

- Pantheon has a bounded `/api/sne/chat` streaming bridge.
- The bridge admits only allowlisted origins and an exact active signed model.
- The bridge forwards only to a fixed loopback SNE child.
- Nexus asserts the exact Pantheon chat URL.
- Ledger list fields are normalized at render time.
- Focused Go tests pass for CORS, preflight, model mismatch, and exact SSE relay.
- Focused Nexus tests pass for identity admission, exact route, SSE completion,
  and incomplete live ledger payloads.

## Claim boundary

This proof promotes the governed browser interaction from unproven to proven.
The four-arm performance matrix, varied-prompt durability, context-length suite,
power/thermal provenance, clean restart, and distributable application gates
remain required before a public SNE v2 performance or launch claim.
