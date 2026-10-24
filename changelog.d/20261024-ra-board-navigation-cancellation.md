### Fixed

- A cancelled Ra board refresh no longer publishes a bundled-reader launch error or a late board after navigation. The previous board, observation time, and error remain unchanged; loading is released and a fresh retry can read normally. Real uncancelled launch, timeout, exit, and decode failures retain their recovery behavior.
- Add actual `loadRouterBoard` entry fixtures for cancelled capture mapping, non-cooperative late success, already-cancelled refusal, and successful retry. These use in-process held reads only, not a router, service, or model. This source correction does not itself establish the exact runtime cause or installed successor qualification.
