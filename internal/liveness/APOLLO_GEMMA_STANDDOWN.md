# Apollo-owned Gemma lifecycle boundary

Apollo's installed router owns its model lifecycle. Pantheon's legacy setup and
healer must not launch a second broker when that installation provides Gemma.
The check uses only the fixed installed helper path under the user's
Applications/Apollo.app, requiring a regular executable leaf, not a directory,
non-executable file, or symlink. An invalid marker returns an error before any
legacy lifecycle action. An absent installation retains existing non-Apollo
behavior, including quarantine/disabled-label protections.

This is stand-down only. No endpoint, model profile, or gemma-server.port is
created or rewritten. The selected host route remains authoritative: M5's
current8520 Apollo router and historical8521 Apollo21 profile are distinct.
This predicate is installation shape, not code-signature, live ownership,
model readiness, or permission to load/switch a model. No health/model request
is made by the predicate. Explicit broker restore remains an operator action;
this change neither restores nor stops a running service.

Regression controls cover invalid marker kinds, selected8520/8521/custom/absent
routes, unchanged profile bytes, and healer refusal to probe/restore on check
failure. Tests use temporary homes and injected probes/serves only.
