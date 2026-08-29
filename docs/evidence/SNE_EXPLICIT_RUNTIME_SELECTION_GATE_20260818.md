# SNE Explicit Runtime Selection Gate

Date: 2026-08-18

## Problem eliminated

The runtime catalog previously keyed packages only by `model_id`. That forced a
new model identity for every copied runtime candidate or prohibited variants
entirely. It also made any future first-match implementation vulnerable to
catalog-order selection.

## Contract

Pantheon now separates stable model identity from optional `runtime_id`:

- a model with exactly one runtime remains backward compatible;
- multiple packages for one model require a unique nonempty `runtime_id` on
  every entry;
- a launch with multiple available runtimes must name `runtime_id`;
- duplicate model/runtime pairs, mixed legacy/explicit variants, ambiguous
  implicit selection, and unknown explicit selection all fail closed;
- lifecycle state retains the selected runtime identity.

## Evidence

Focused tests passed:

`go test ./internal/sne ./internal/dashboard`

The adversarial catalog test proves that two variants cannot be selected
implicitly and that mixed legacy/explicit entries cannot load. Existing
single-package catalogs remain valid. No production catalog was changed and no
candidate was promoted.
