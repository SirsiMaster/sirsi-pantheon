### Corrective source candidate — Gemma compatibility census identity

- Attribute the retained `sirsi-gemma-worker.sh` census matcher to the declared
  `gemma-pantheon` worker lane, matching the published native compatibility entrypoint.
- Pin that identity in the existing already-tracked-worker census fixture.
  GPU-server/triage matchers, process matching, registration mechanics, native
  worker ownership and draft-only policy are unchanged. No shell worker logic
  is restored; no claim is made that the exec wrapper remains resident.
- Extracted from the preserved primary checkout. Focused normal/race census
  fixtures pass; restoring the released matcher fails the new identity assertion.
  Installed qualification and release integration remain separate; no service,
  registry or primary-checkout changes were performed.
