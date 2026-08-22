# SNE Governed Model Removal Candidate

**Date:** 2026-08-20  
**Status:** Source and focused CPU gates passed; clean-host package gate pending  
**Scope:** Daytime-safe source work only; no model data was removed and no GPU workload ran

## Product contract

Pantheon must let a user remove an installed SNE model without deleting a model
that is running, confusing one catalog identity for another, or collecting bytes
still shared by another installed model.

The candidate implements that contract in two layers:

1. `sne-model-remove` resolves the exact governed model ID from the SNE model
   catalog, applies the selected readiness policy, and delegates removal to the
   existing transactional model store.
2. Pantheon accepts only an exact admitted `catalog_entry` and `model_id` pair,
   requires the SNE lifecycle to be stopped, invokes the helper with explicit
   arguments rather than a shell, and returns the native removal receipt.

The existing transactional store remains authoritative. It moves the governed
view through its recovery transaction, removes only objects at their final hard
link, retains shared objects, and can finish interrupted removals during model
store recovery.

## Fail-closed behavior

- Unknown or mismatched catalog identities are rejected.
- Cross-origin browser mutations are rejected.
- Missing lifecycle state is treated as unsafe rather than assumed stopped.
- Starting, ready, stopping, recovering, or failed runtime states must be
  resolved to `stopped` before model removal.
- A missing or non-executable removal helper is an actionable configuration
  failure; Pantheon does not fall back to ad hoc filesystem deletion.
- Removal does not mutate runtime packages or immutable SNE releases.

## Candidate files

- `sirsi-native-rebuild/cmd/sne-model-remove/main.go`
- `sirsi-pantheon/internal/dashboard/sne_install.go`
- `sirsi-pantheon/internal/dashboard/server.go`
- `sirsi-pantheon/internal/dashboard/sne_install_test.go`

## Remaining admission gates

1. Carry `sne-model-remove` through the signed SNE package and Pantheon install
   layout alongside checkout and recovery.
2. Expose a separate accessible **Remove model** control without displacing
   **Start model**.
3. Verify exact command arguments, same-origin enforcement, active-model
   refusal, final-object collection, shared-object retention, interrupted
   recovery, reinstall, and clean uninstall.
4. Record package hashes and clean-host M1/M5 evidence before promotion.

Until these gates pass, governed model removal is implemented source work, not
a completed user-facing release capability.

## Focused verification

- `go build ./cmd/sne-model-remove`: passed.
- `go test ./internal/modelcheckout`: passed.
- `go test ./internal/dashboard`: passed.
- `zsh -n scripts/assemble-sne-product.zsh scripts/verify-sne-product.zsh packaging/tools/install.zsh`: passed.
- Product Doctor now fails if checkout, recovery, or removal is absent.
- Pantheon measures all three installed helpers before enabling removal. The
  UI and privacy-safe diagnostics expose only readiness or an actionable
  repair/reinstall message, never filesystem paths.
- The real served Pantheon page has a regression gate for a separate Remove
  model control, Enter/Space activation, accessible naming, shared-object
  disclosure, and reinstallability language. This is static accessibility
  evidence; hands-on VoiceOver remains a clean-host gate.
- The shared Pantheon shell now gives action/navigation controls a visible
  focus ring, honors reduced-motion preferences, and strengthens focus/text
  under increased-contrast preferences. The served-page regression gate covers
  these contracts; assistive-technology validation is still pending.
- The external lifecycle evidence harness now requires all helpers in both GA
  inputs and rechecks them after update and rollback.
- Static lifecycle parsing exited successfully. The managed runner printed a
  non-fatal `nice(5)` permission warning; no lifecycle or inference campaign
  executed, so this is not counted as runtime evidence.

The package authority now installs all four required executables together:
`sned`, checkout, recovery, and removal. Pantheon resolves every helper from
the installed SNE package root under Application Support rather than relying on
an unrelated `~/.local/bin` side installation. This prevents a clean-host
package from appearing complete while lifecycle helpers are absent.
