### Fixed

- **`/api/router` sends an empty list, not `null`, for a release section with no entries.** Right after a release cut `[Unreleased]` is empty, and its `items` serialized as JSON null; a client iterating it had to special-case it. Test added.
