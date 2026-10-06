### Fixed

- **Sending to a lane now validates against the pinned registry.** `sirsi router registry sync` pinned every reader to origin/main except the send path, which still read the shared working tree. A lane declared on origin (claude-m5-compasspoint, after #1023) was refused as "not fully declared" on any host whose working tree was behind. Dispatch now reads the same source as the rest of the router.
