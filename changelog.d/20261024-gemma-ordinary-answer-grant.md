### Fixed

- Ordinary native Gemma workers now consume the existing lead-owned answering
  grant before provider preflight and stamp/read back that grant on their own
  registered thread before claiming or generating, matching the tool-turn path.
  Missing or unreadable declarations fail closed; no registry, service policy,
  role authority or model lifecycle is changed.
- An answering-authority refusal after generation now fenced-blocks the same
  original with its exact draft for lead review instead of requeueing inference
  until dead-letter. Already-published nonbinding replies still require explicit
  correlation/reconciliation. Block failure reports both errors and the draft.
  Existing dead letters are not reopened or falsely declared completed.
- Added isolated startup-grant/stamping and authority-refusal regression fixtures.
  Source fixtures are not installed worker, model, hosted/all-lane or delivery
  qualification; normal verification and release integration remain separate.
