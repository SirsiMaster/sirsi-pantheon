### Fixed

- Router sessions with a registered thread now read, publish and invalidate only their own per-thread cache. Threadless callers no longer borrow a thread-bound same-agent session from the shared legacy cache. Primary and coordination sessions cannot clobber each other's lease identity through that alias. Server lease ownership checks are unchanged; restoring real native provider markers remains required, and legacy threadless leases are not silently transferred.
