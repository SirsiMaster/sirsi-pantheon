- **fix(maat):** the conflict report now names the lanes that held a live floor
  share when it was sampled. Its doc comment promised that a floor lane's
  activity is "never a contaminant", but a process counts as shared only when
  the probe attributes an owner and the default probe attributes none, so a
  floor lane's processes were intruders and invalidated a quiet or loaded hold.
  That outcome stays (any foreign load contaminates a quiet measurement) and the
  comments, README, user guide and ADR-120 now say so. New `floor_holders` field
  (holder, work, reservation, cores) on `sirsi maat conflict-check --json`, a
  matching line in the human output, and an invalidating summary ends with
  "; floor shares live at sample time: ..." (five named, then "+N more"). It is
  co-presence only; nothing is attributed. Refs: maat-conflict-probe-owner-attribution, ADR-120.
