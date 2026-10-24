### Fixed — root relay observed spool argument variants

- Accept only the two observed literal six-argument profiles ending with
  `/var/sirsipantheon/relay` or `/private/var/sirsipantheon/relay`.
- Do not normalize paths, resolve aliases, rewrite plists, or change flags,
  authorization, root/artifact/instance bindings or the retained physical-spool
  no-follow/quiescence probe.
- Add explicit positive profile and lookalike/escape/alias refusal fixtures using
  existing native doubles. Source-only; main owns admitted qualification.

Refs: existing Studio125 root-relay refresh defect; ROOT_RELAY_REFRESH.md;
AGENTS.md A35. No live root read, service action or rollout proof claimed.
