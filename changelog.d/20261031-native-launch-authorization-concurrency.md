### Fixed

- The existing native process-launch authorization callback is explicitly MainActor and Sendable across remediation, relay maintenance and app-update adapters. Its fresh authorization still runs on the main actor immediately before launch.
