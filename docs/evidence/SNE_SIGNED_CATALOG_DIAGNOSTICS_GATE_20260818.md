# SNE Signed Catalog Diagnostics Gate

**Date:** 2026-08-18  
**Scope:** Pantheon product diagnostics and shared SNE read model  
**Performance claim:** None

**Workspace mirror:** https://docs.google.com/document/d/1qzn076pztRI3cbeyymIPc3I7YgFurHYhs47THLdlitg

## Why this gate exists

Earlier package failures demonstrated that a unit can be correct while the
installed product is wrong because a dependency, path, signature, catalog, or
runtime identity is lost at an integration boundary. Pantheon must therefore
show the exact governance state it actually uses rather than deriving a green
status from a responsive process.

## Implemented contract

Pantheon's shared SNE read model and terminal UI now expose:

- catalog verification state;
- whether a detached signature is required;
- authenticated catalog ID;
- immutable current catalog SHA-256;
- authenticated entry count;
- retained catalog-version count;
- rollback availability;
- active runtime ID in lifecycle state.

Signed runtime variants expand into explicit model/runtime rows. Each row shows
its `runtime_id`, and start requests submit both the stable `model_id` and the
selected runtime identity. Catalog order is never a selection rule.

Invalid signature, missing trust material, unreadable atomic pointer, or catalog
store failure is rendered as invalid with the underlying diagnostic. It is not
silently converted into an empty or healthy catalog.

## Executed gates

```text
go test ./internal/sne -run TestSignedRuntimeCatalogStoreUpdateRollbackAndRemoval
PASS

SNE_REAL_DEFAULT_SIGNED_CATALOG=1 go test ./internal/dashboard \
  -run TestRealDefaultSNELifecycleLoadsSignedCatalog -v
PASS
```

The real default gate accepted exactly:

- catalog ID: `sne-gemma4-v1`
- current version: `bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d`
- entries: `11`
- retained versions: `1`
- rollback available: `false`
- signed catalog required: `true`

One aggregate verification command was initially invoked from the SNE repository
instead of the Pantheon repository. Go rejected the nonexistent package paths;
the chained mirror step did not run. This is classified as a harness failure,
not product evidence. It was rerun from the owning Pantheon module and both full
package suites passed. Rule 30 now requires module/package ownership preflight.

The aggregate dashboard suite contains loopback `httptest` servers. Its first
restricted run hit the known bind denial; the approved host-boundary rerun
passed `internal/dashboard` and `internal/sne`, followed by the real signed
catalog gate. The denial remains harness evidence, not a product failure.

Pantheon now lists retained immutable versions and provides keyboard-accessible,
confirmation-gated rollback and inactive-removal actions. Both endpoints are
same-origin POST operations and use the full SHA-256. SNE must be stopped;
`TestSNELifecycleTransitions` proves rollback is rejected while ready. The
underlying store continues to reject active removal, unexpected files, invalid
digests, and signature/package verification failures.

The first new lifecycle test build omitted the standard-library `strings`
import used by its assertion. Compilation failed before test execution, the
import was added, and the unchanged full suites passed. This is retained as a
test-construction failure and not represented as a runtime or product defect.

## Failure-prevention lesson

Manager-only tests are insufficient. Product admission requires the same
authoritative state to survive manager, read-model, API, and UI boundaries.
This is permanent SNE Rule 32. No serving-speed, model-quality, physical-memory,
or GPU-utilization claim follows from this gate.

## Live M5 product projection

An isolated `sirsi dashboard --no-open --port 9120` instance served the real
`/api/sne` read model on the Apple M5 Max and was then shut down cleanly.
Observed runtime-catalog state was:

- `state=verified`
- `signed_required=true`
- `catalog_id=sne-gemma4-v1`
- `version_sha256=bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d`
- `entries=11`
- `versions=1`
- `rollback_available=false`
- `update_feed_configured=false`
- lifecycle `stopped`

The separate model-admission library exposed 16 tuples. Tuples lacking a
verified packaged runtime remained disabled with an explicit reason; Pantheon
did not treat model installation as runtime-package availability and did not
fall back to another model, runtime, precision, or framework.
