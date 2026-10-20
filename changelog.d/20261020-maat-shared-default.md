### Fixed

- Ma'at ordinary launches default to shared CPU build work rather than an
  implicit loaded/full-share model request. A cooperative one-CPU default can
  be increased explicitly. Quiet measurements and loaded/model work remain
  explicit, and existing jobs are not stopped or relabeled.
- Raw CLI reservations default to build rather than quiet. This retains the
  existing client/server schema and lease/ownership checks.
