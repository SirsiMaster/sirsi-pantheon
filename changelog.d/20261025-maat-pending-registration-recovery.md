### Fixed

- Ma’at retains the safe cause class of an uncertain executor registration and
  observes only its generated thread ID within the existing bounded recovery
  deadline. Pending absence no longer discards the original cause or triggers
  another registration. Denial, terminal/runtime mismatch and foreign identity
  checks remain immediate; existing healthy execution leases are unchanged.
