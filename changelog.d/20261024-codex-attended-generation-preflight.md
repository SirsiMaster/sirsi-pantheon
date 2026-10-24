### Fixed

- Add explicit `thread heartbeat --ensure` for the existing registered Codex
  attended automation boundary: strict current durable-host generation proof,
  normal own-scope successor registration/readback and persisted binding before
  fenced heartbeat. Ordinary heartbeat and terminal/foreign identity fences are
  unchanged. Inert source fixtures cover A-to-B ordering and conservative refusal.
  No automation installation, service action, model invocation or installed
  restart-survival qualification is claimed.
