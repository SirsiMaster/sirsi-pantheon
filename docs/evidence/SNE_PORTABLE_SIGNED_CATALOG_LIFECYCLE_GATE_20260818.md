# SNE Portable Signed Catalog Lifecycle Gate

Date: 2026-08-18

## Result

Pantheon's live M5 configuration was migrated from a signed host-absolute
catalog to the signed portable catalog. Rollback copies of the previous catalog
and signature were retained. The default product lifecycle then launched and
stopped the admitted plain Gemma 4 12B affine-8 model successfully.

- catalog ID: `sne-gemma4-v1`
- entries: `11`
- portable catalog SHA-256: `bbcdbaf9f86b75e58768c7c567f0f5389b4aad88d601e5e5bfc78998df70a35d`
- signature SHA-256: `6842cee0dbcc6d36be5f59e6eedb819027e378d001afb208a4e9fe24f2314351`
- public key SHA-256: `60a94a48404769b133589e9b23ba137a778789b037c36af48e5f4fb6cf8b9808`
- model: `gemma-4-12b-it-affine8-sne-q8-plain-v1`
- runtime SHA-256: `09f5038bc8080b37bc99b46c6e9d6080facbd533213c263cca37fdb64dd59261`
- artifact set SHA-256: `5a3ca87acaaec16ef3c1299322767d121fdcf706bf98c33a6e6a8af7da2c6b07`
- verified files: `8`
- verified bytes: `12,935,379,671`
- loaded MLX: package-local `lib/libmlx.dylib`
- lifecycle: start, readiness, clean stop passed

This proves the real M5 lifecycle through the portable signed catalog. It does
not prove M1 package availability, clean-host installation, inference quality,
or performance. The M1 Tailscale SSH attempt timed out before transfer, so no
remote state changed and cross-host proof remains open.

## Harness lessons

Real test inputs must be absolute because Go executes package tests from the
package directory, not repository root. Listener-bearing Pantheon suites must
run in the approved host boundary rather than the restricted sandbox. Both
conditions are now explicit in the real gates; their failed receipts are not
classified as runtime or GPU failures.
