### Fixed

- Put the inert ask-generation fixture helper's error return last and adjust
  its callers to satisfy ST1008. No production code, test inputs or assertions
  change. The prior b3 qualification remains retained as failed at lint; this
  source-only correction does not claim a new qualification result.
