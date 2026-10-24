### Fixed

- Guard the attended generation successor in the normal registration retry path:
  occupied conflicting/retired IDs refuse without overwrite; exact live duplicate
  transitions reuse readback, and absent targets use insert-only CAS. The existing
  RPC carries a versioned restrictive argument that old decoders refuse, with no
  permission or ordinary-upsert fallback.
- Apply explicit own-session ensure at both heartbeat and wait entry. Add private
  real-Cobra/scoped-handler fixtures for generation transition, registration and
  wait ordering, conflicts/races, real markers, terminal preservation and explicit
  environment binding consumption. No tests/builds, deployment, installed
  automation adoption or original restart-survival completion claimed.
