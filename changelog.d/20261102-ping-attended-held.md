### Fixed
- `sirsi router ping` now reports a declared-attended lane whose session is not open as HELD (needs-owner) instead of WATCH_ONLY "declares no working consumer". The loop withholds its consumer on purpose for attended lanes, so the old wording sent people chasing a broken consumer.
