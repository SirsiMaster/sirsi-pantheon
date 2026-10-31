### Fixed

- The native app answers the local sibling scanner’s exact `version --json` request before app startup, using its own bundle stamp and executable URL. Missing, foreign or inconsistent identity still refuses; inactive-case completeness checks are unchanged.
