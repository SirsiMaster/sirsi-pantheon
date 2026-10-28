### Fixed

- `sirsi router ping` no longer reports a lane whose background loop is quarantined as "needs a human". That verdict is reached only when no attended session is heartbeat-fresh, so it now says the background loop is quarantined and no attended session is registered. A working session whose thread was reaped (a resumed session with a new PID) must re-register to be seen.
