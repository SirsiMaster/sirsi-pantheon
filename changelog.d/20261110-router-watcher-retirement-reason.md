# Unreleased — exact watcher retirement reason

Report an exact-incarnation cooperative watcher stop as a normal exit, including
requests arriving during observation setup. Missing, malformed or replaced custody
and ancestor/subscription failures remain errors; valid stale requests do not stop
a successor. Close the owned observation monitor on setup refusals as well as
normal retirement. No consumer is signaled or restarted.

Preserves the original `32b28d04` correction and adds the independently identified
setup-gap control. Qualification remains pending on this source successor.

Refs: ADR-024, ADR-036, ADR-037.
