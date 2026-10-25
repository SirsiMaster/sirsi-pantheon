### Fixed

- Offer existing local memory initialization/recovery inside the native managed
  app-update flow, retain the selected update/rollback through recovery, and
  require verified store readback plus fresh confirmation before the canonical
  request. Reuse engine-owned operation and memory models; no error-text parser,
  store reset, automatic retry or Installer-success inference. Preserve the
  separate labelled legacy app/helper recovery Installer fallback.
