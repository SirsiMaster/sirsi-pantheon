### Fixed
- Make the transient registry-read observer fixture deterministic by isolating it on legacy storage and holding its owned root read-only while malformed bytes are written through the existing registry file descriptor. The fixture distinguishes a permission-denied heartbeat write from an observed JSON parse failure and restores the original snapshot before releasing the barrier.
