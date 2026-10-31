### Fixed

- Trash browsing relies on the directory session’s existing descriptor-owning destructor instead of reading actor-isolated session state from an actor’s nonisolated destructor. Explicit close, cancellation fencing and release of all owned descriptors remain unchanged.
