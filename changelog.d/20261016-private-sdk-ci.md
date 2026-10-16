### Fixed
- Trusted Pantheon CI builds can read the two private Apollo SDK repositories using separate read-only deploy keys. Forks and untrusted events are refused; temporary credentials are removed on exit. No Mac, signing or binding credentials are reused.
