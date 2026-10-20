### Fixed

- Preserve explicitly configured, token-scoped logical cloud executor identity
  in client sessions and thread registration, heartbeat, save, and close paths.
  Reject malformed names or conflicting physical-machine identity before HTTP;
  keep server host-token and lane-scope authorization unchanged. Native Mac
  host behavior and resident-worker protection are unchanged.
- Include the CLI's routerstore import so the registration command compiles;
  package-only router tests do not qualify the command entry point.
