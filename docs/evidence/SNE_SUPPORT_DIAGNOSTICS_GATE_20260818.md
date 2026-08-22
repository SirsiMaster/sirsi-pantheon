# SNE Support Diagnostics Gate — 2026-08-18

**Classification:** Pantheon commercial-product operations evidence  
**Canonical source:** `sirsi-pantheon/docs/evidence/SNE_SUPPORT_DIAGNOSTICS_GATE_20260818.md`  
**Owner mirror:** `Desktop/Sirsi - Owner Reading Room/SNE/SNE_SUPPORT_DIAGNOSTICS_GATE_20260818.{md,html}`  
**Workspace mirror:** [SNE Support Diagnostics Gate — 2026-08-18](https://docs.google.com/document/d/1aTpS5E5qrYhtWL-BIu0eqM4FwcwRC_i_8sldnbCXIug)

## Outcome

Pantheon now exports one privacy-safe, machine-readable SNE support artifact from `GET /api/sne/diagnostics`. The dashboard exposes the same capability as **Export privacy-safe support diagnostics**. The response is an attachment, is marked `Cache-Control: no-store`, and uses schema `pantheon.sne-support-diagnostics.v1`.

The export deliberately reconstructs a curated support view. It does not serialize raw internal structs or errors. It includes:

- Apple device family, platform architecture, and unified-memory capacity;
- SNE service and lifecycle state;
- exact model and runtime identities;
- signed runtime-catalog state, catalog ID, complete SHA-256 version, retained versions, and rollback/update availability;
- governed model tuple summaries without local package paths;
- Pantheon's authoritative current RAM, swap, reserve, and pressure measurements.

It excludes prompts, generated text, assistant contents, API keys, private keys, environment variables, process command lines, home-directory paths, temporary paths, and raw errors that may contain those values.

## Executable gates

Focused package gate:

```text
go test ./internal/sne ./internal/dashboard
ok github.com/SirsiMaster/sirsi-pantheon/internal/sne
ok github.com/SirsiMaster/sirsi-pantheon/internal/dashboard
```

Full Pantheon gate:

```text
go test ./...
PASS (all packages and tests/e2e)
```

The privacy regression test injects `/Users/...`, `/private/tmp/...`, `DYLD_LIBRARY_PATH`, and `api_key` strings into fields intentionally omitted from the support schema. It then fails if any injected value or forbidden data-class label appears in serialized output. It also requires the exact catalog, runtime, model, device, and version identities.

## Real M5 product-boundary proof

An isolated Pantheon dashboard was launched on the M5 Max and the actual attachment endpoint was downloaded over loopback. The response declared:

```text
Content-Type: application/json
Content-Disposition: attachment; filename="sirsi-sne-diagnostics-20260818T083722Z.json"
Cache-Control: no-store
```

The decoded artifact reported:

```text
schema: pantheon.sne-support-diagnostics.v1
platform/architecture: darwin/arm64
device: Apple M5 Max
service: stopped
catalog state: verified; signed required
catalog: sne-gemma4-v1
catalog SHA-256: bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d
runtime entries: 11
governed model tuples: 16
RAM: 51,539,607,552 bytes total; 35,662,168,064 bytes available
swap: 509,733,765 bytes used
pressure: normal (bootstrap-snapshot)
```

The serialized-byte scan passed with no home paths, private temporary paths, dynamic-linker variables, credential labels, prompt fields, generated-text fields, or assistant-weight paths. The isolated dashboard was then terminated cleanly.

## Permanent prevention rule

**Rule 35 — Curated support evidence, never raw state:** Product diagnostics must be constructed from an explicit versioned allowlist. Raw internal structs, errors, environments, command lines, user content, and filesystem paths may never be serialized into a support export. Every schema revision requires both positive identity assertions and negative sensitive-data injection tests, followed by a real product-boundary download gate.

This rule converts the prior recurring class of hidden environment, ephemeral path, and package-identity failures into support evidence without turning diagnostics into a new privacy or credential leak.
