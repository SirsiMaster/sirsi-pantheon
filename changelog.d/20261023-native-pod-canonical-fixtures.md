## Native pod qualification

- Resolve only owned temporary fixture roots to their canonical macOS path before journal/cache tests; keep production no-follow traversal unchanged. Add a real canonical-parent success control and a symlink-ancestor refusal fixture so rejection cannot pass merely because `/var` aliases `/private/var` on a developer Mac.
