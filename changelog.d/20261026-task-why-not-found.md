### Fixed
- **`sirsi router task why` on an unknown task now reports "not found" instead of an HTTP 500.** `TaskEligibility` returned a plain error string, which the store service could not match to a known error, so it answered with a 500 server error. It now wraps `ErrNotFound`, so the client receives the named not-found error. Refs: ADR-062.
