# Installed Apollo app transport

`installed_app.go` is the narrow Qwen2.2.6 bridge alongside (not in place of)
the signed native fleet backend. One compiled typed profile pins the model,
precision, checkpoint/tokenizer/template and five installed artifact hashes.
`InstalledApp` observes fixed install locations without changing a registry.
`ChatInstalledApp` accepts no transport/configuration override, disables proxies
and redirects, and dials only literal loopback8520. Apollo owns live admission.

`chatInstalledApp` is a private transport seam for synthetic CPU fixtures.
The public path validates artifact identity before any HTTP request. Responses
are buffered/size-bounded and bind exact model, backend, requested tier, drafter
presence and completed text choice before exposing content. No POST retry,
fallback, worker launch, native-fence claim or tool execution is permitted.

Read `docs/user-guides/apollo-installed-qwen.md` for mode semantics and adoption
limits. Source authority: Rail103939 binding226 at engine184ca7e; parent SSA
authorized this exclusive additive bridge, leaving Main/Pantheon other edits
disjoint. No engine performance or fleet certification is implied by fixtures.
