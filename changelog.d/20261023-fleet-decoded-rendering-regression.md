### Fixed

- Retain the original native-fleet-state-210624 uppercase decoded/rendering
  regression against the real Fleet row. Extract the existing row unchanged
  so fixture rendering does not mount Fleet's live CLI task. Cover all six
  canonical states plus unknown as synthetic data only; no appearance,
  producer, state-mapping, or installed all-state qualification change.
