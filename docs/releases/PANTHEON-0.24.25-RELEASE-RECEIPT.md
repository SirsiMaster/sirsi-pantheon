# Sirsi Pantheon v0.24.25 release receipt

Date: 2026-09-28

This commercial patch release records the SNE consumer-authority boundary in
the Pantheon router registry. The legacy `codex-sne-runtime` and
`claude-inference` prompts now direct SNE source, recipe, runtime, benchmark,
qualification, and promotion work to the authoritative `codex-inference` lane
under `SNE_SOURCE_OWNERSHIP_AND_CHANGE_POLICY.md`.

## Exact source and validation

- Release candidate source: `e64e27ad83b7936f9896697031acff30424258a7` / tree `6508899111137f5657ecdd8cdc91edc38e205d9a`.
- Base release: `v0.24.24`.
- The change is routing guidance only; it does not mutate SNE source, runtime,
  model, host, or qualification state.
- CI, GoReleaser, signing, notarization, cask, and package readback remain the
  authority for published artifacts.

## Boundaries

This receipt does not claim SNE qualification, model serving, installed-host
evidence, or live router mutation.
