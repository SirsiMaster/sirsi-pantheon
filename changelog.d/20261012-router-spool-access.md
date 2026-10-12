### Fixed

- Router relay clients now distinguish denied or failed slot creation from actual request saturation. Permission and sandbox failures retain their underlying error and explicitly report that no request was published.
- Stale slot recovery now returns removal errors and performs only one bounded reacquisition instead of potentially looping indefinitely.

### Operations

- Added a restricted Codex relay qualification recipe using a named permission profile, the canonical real system spool path, and disabled direct networking. This is a client access recipe, not a second router database or a claim of completed fleet rollout.
