### Fixed

- Honor explicitly zero Thoth compact retention limits without applying the
  implicit keep-20 journal policy. Omitted limits retain the legacy default;
  positive count limits retain their existing behavior. Explicit zero/zero uses the
  Go implementation because npm delegation cannot preserve that intent.
- Compare journal retention by local civil calendar dates, retaining the exact
  N-day cutoff. Fresh entries no longer expire across UTC midnight or a DST
  boundary because their date-only headers were interpreted as UTC instants.
- Route positive-age compact policies through the corrected Go writer even
  when an external npm writer is installed. Implicit-default and count-only
  delegation remain unchanged. These policies govern journal retention, not
  the separate memory.yaml session-decisions cap.
