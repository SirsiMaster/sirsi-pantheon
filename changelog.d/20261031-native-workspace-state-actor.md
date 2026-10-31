### Fixed

- Native workspace state and selection callbacks explicitly use the main actor. Scroll bindings retain the same navigation/owner-generation fence using a sendable identity value, and test cleanup captures only its private suite name.
