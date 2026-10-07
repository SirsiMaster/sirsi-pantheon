### Fixed

- **Wake loops share one CPU-headroom probe per host.** Every loop ran `top -l 2` each cycle; nine loops cost 0.1-0.15 of a core continuously (measured by Mercury on the M1 while it was CPU-bound receiving at ~12 GB/s). One probe per host per 30 s now serves all loops through `~/.sirsi/host-load.cache`; the dispatch gate's behavior is unchanged.
