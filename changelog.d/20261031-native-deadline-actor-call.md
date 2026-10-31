### Fixed

- The Ma’at history deadline task calls its synchronous main-actor deadline transition directly while preserving the asynchronous completion callback and cancellation behavior.
