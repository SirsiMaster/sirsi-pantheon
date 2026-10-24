### Fixed

- Correct the a622 source-review findings: map optional `json_object` from the
  existing selected Pantheon profile to three-state capability evidence for
  that exact endpoint/model, and preserve `--route` consent when measured
  alarms stop model inference. Format/determinism flags alone do not construct
  the provider ladder or bypass the heuristic stop. Inert fixtures cover the
  real profile/resolver/Cobra/HTTP path and selection isolation. No installed
  profile, SNE schema/service, credential or deterministic wire mode changes;
  qualification remains unrun at this source checkpoint.
