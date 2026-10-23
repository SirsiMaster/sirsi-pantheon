### Fixed
- Add a read-only-by-default, explicitly granted single-service launchd lifecycle
  path with pinned runtime/artifact identity and honest restart readback. This
  avoids using a bulk fabric refresh for one Apollo or Mercury helper. No
  automatic elevation, global stop-guard relaxation or live service mutation.
