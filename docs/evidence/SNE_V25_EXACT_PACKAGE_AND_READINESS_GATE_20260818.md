# SNE v25 Exact Package and Readiness Gate

Date: 2026-08-18

## Scope

Pantheon evaluated the copied SNE v25 candidate package without changing the
production runtime catalog:

`sne-v2-api-paged-v25-package-v2/SNE-2.4.1-12b-affine8-paged-v25-candidate-v2-darwin-arm64`

The target was the transactionally installed Gemma 4 12B affine-8 checkpoint.
No immutable SNE release was modified.

## Exact accepted identity

- runtime SHA-256: `f967611cb3ba55d90001aa190bb7ef1dc8176a085b303912d5b6aa67203418cc`
- model manifest SHA-256: `931bc842e6a71936588e3bdd61cf100e67dc9254e200655dac06545ccc17864c`
- model ID: `gemma-4-12b-it-affine8-sne-v25-multisession-lru-candidate`
- service version: `0.1.0-dev`
- API version: `v0`
- profile: `interactive`
- verified artifact-set SHA-256: `c6c315c9d26e7b353b65c9544c611f19eafb59b9a0f917aaed464b6582571a87`
- verified artifact bytes: `12,935,160,119`

## Gate sequence

1. Focused `internal/sne`, `internal/dashboard`, and supervisor command tests
   passed.
2. A real Metal process loaded only the copied package's pinned `libmlx.dylib`.
3. Pantheon required HTTP readiness plus exact status/model identity.
4. Pantheon parsed every governed Mach-O dependency and `LC_RPATH`.
5. The copied package passed the final self-containment and readiness gate.

## Preserved failed receipts and corrections

The first real package-boundary attempt rejected the package's intentional
`lib/mlx.metallib` symlink. Inspection showed that the link resolves to the
regular `share/mlx.metallib` inside the same package. The invariant was refined:
contained regular-file symlinks are permitted; broken links, directory links,
and package escapes remain forbidden.

The second attempt applied a regular-file rule to `LC_RPATH`, even though an
rpath names a directory. The parser now distinguishes dependency files from
rpath directories and accepts only real, non-symlinked directories contained
inside the package.

A macOS `/var` to `/private/var` canonicalization difference was also caught by
the unit fixture. Pantheon now resolves the package root before symlink
containment comparisons.

These are harness/admission corrections, not GPU-performance results.

## Product boundary

This result proves package self-containment and exact Pantheon readiness for the
copied v25 candidate. It does not promote v25 into the production catalog, does
not make a 59 tok/s claim, and does not replace the remaining session-capacity,
resource, Nexus, four-arm, soak, signing, notarization, and clean-host gates.

