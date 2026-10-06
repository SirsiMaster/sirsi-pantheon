### Fixed

- **The release train clears the abandoned branch a stopped run leaves behind, and refuses to touch real history.** A run that stopped before its push (a failing test, a lint error) left a local `release/<version>` branch and a temp worktree, and the next attempt died on "worktree failed" because the branch already existed. The train now removes such a leftover only when it was never pushed, has no PR in any state and holds nothing beyond its own prep commit; otherwise it stops and says what to inspect.
