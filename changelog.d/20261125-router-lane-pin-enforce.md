### Added
- Opt-in enforcement seam for the lane-to-Mac pin (ADR-065 rs-37, option a'). New service-side flag
  `SIRSI_ROUTER_LANE_PIN_ENFORCE` (read once at `sirsi router serve` start; unset or false = today's report-only
  behavior exactly; a non-boolean value is a startup error). When on, a served `Wait` whose token's adopted machine
  id differs from the lane's pinned machine is refused with 403 `ErrLanePinMismatch` (message names the lane, the
  pinned machine and the token's machine; no token value). Only an exact mismatch refuses: unpinned lanes, tokens
  with no machine id, and evaluation errors never do. `RemoteStore.ListenNotify` long-polls `Wait`, so it is covered.
  No schema change, no new verb or credential. Turning the flag on is an owner decision.

Refs: ADR-065, docs/design/ADR065_INFORMER_AUTHORIZATION_DECISION.md
