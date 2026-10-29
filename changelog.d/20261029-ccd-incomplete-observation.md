### Fixed
- Refuse CCD reap/archive before mutations when initial process observation fails, is partial/malformed or cannot establish a runner's session attribution. Preserve successful no-match behavior and existing archive grace/retention/live-session rules; do not treat process visibility failures as proof of absence.
