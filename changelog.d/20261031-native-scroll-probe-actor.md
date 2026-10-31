### Fixed

- The Ma’at AppKit scroll coordinator and its existing main-queue callbacks explicitly retain main-actor ownership, without adding task hops or changing restoration, observer cleanup or stale-owner fences.
