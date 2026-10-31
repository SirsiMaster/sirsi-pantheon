### Fixed

- Add an explicit relay duty-drain API that joins owned lane workers, active
  forwards and serial outbox retries after discovery stops. Its bounded timeout
  reports incomplete custody rather than success and never cancels or replays
  an admitted mutation. Preserve `Serve`'s existing worker-join behavior and
  add a separate bounded `ServeDrained` result; reject reuse of a drained relay
  instance.
- Retain consumed requests when their response cannot be published, report
  settlement/cleanup debt through the explicit drain, and keep unparkable
  unknown outcomes outside the automatic retry queue.

This is the first source checkpoint in the existing native lifecycle plan, not
an installed service refresh, producer-ingress fence or legacy migration proof.
