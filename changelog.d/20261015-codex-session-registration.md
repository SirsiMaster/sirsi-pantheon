### Fixed

- Native Codex sessions now key router registration and Ma'at Phase-1 attribution on `CODEX_SESSION_ID` or `CODEX_THREAD_ID` without posing as Claude. Conflicting Codex aliases refuse attribution; unregistered or undeclared agents still refuse. Claude's existing explicit session binding and shared marker paths are unchanged. This is local registered-marker attribution, not a new authentication or signing grant.
