### Fixed

- Validate the observed kmutil loaded-kext table header and every row before
  reporting RDMA-family presence or absence. Match the exact family bundle ID;
  malformed successful output and command failures remain unknown with a reason.
  Permit only the exact observed release-variant notice before the table.
  This read-only telemetry correction changes no NVRAM, enablement, hardware
  support, or transfer qualification. Fixtures preserve observed native table
  shape with live load addresses replaced by representative constants.
