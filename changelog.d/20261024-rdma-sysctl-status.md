### Fixed

- Preserve the RDMA legacy sysctl command status rather than treating nonempty
  failure diagnostics as loaded-family evidence. Trust only successful output
  matching the retained historical date/time and RDMA_family version shape;
  unfamiliar output falls back to the validated kmutil inventory. This is a
  conservative compatibility recognizer, not a newly verified native format.
  Shared command handling, NVRAM reporting and device qualification are unchanged.
