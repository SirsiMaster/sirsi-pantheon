### Fixed

- Add an explicit relay duty-drain API that joins owned lane workers, active
  forwards and serial outbox retries after discovery stops. Its bounded timeout
  reports incomplete custody rather than success and never cancels or replays
  an admitted mutation. Preserve the existing fast-cancel API as a distinct
  non-drain contract; reject reuse of retired per-instance lane channels.

This is the first source checkpoint in the existing native lifecycle plan, not
an installed service refresh, producer-ingress fence or legacy migration proof.
