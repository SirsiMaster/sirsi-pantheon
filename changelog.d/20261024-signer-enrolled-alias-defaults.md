### Fixed

- Use the existing pinned `sirsi-m5` and `sirsi-m1` SSH aliases as ordered signing
  client defaults instead of DHCP addresses. Preserve explicit host overrides,
  strict noninteractive transport, enrolled-service checks and signature checks;
  do not enroll hosts, change trust or launch signing services. Denial-only
  causal fixtures cover unset/empty/default/override selection and reject the
  former DHCP defaults without contacting any signer.
