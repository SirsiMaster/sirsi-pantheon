### Fixed

- Preserve the existing no-complete-commercial-release update/recheck promise
  when composing the additive whole-app rollback recipe. Keep its existing
  installed-product preservation behavior, source guard and all rollback outputs.
  This corrects recipe metadata; no update, Installer or runtime behavior changes.
