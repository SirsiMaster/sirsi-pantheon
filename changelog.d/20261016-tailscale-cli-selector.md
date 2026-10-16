### Fixed
- **guard**: `checkTailscaleBackend` now sets the vendor-documented
  `TAILSCALE_BE_CLI=1` override on Darwin before invoking the Tailscale CLI,
  so automation contexts (launchd, no TTY/SHLVL) can't be heuristically
  routed into the GUI instead of returning CLI status JSON. Non-Darwin
  platforms are unaffected.

Refs: PANTHEON_RULES.md §2.3 (A6), §A16; internal/guard/network.go
Changelog: v0.24.106 — Tailscale Darwin CLI selector
