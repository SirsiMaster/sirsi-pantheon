# Installed Apollo Qwen bridge

Platform-foundation source increment, not an installed-client or runtime certification.

The normal command selects the exact installed Qwen route with explicit precision
and mode. After normal reviewed, tested and signed Pantheon adoption:

```sh
sirsi apollo catalog --json
sirsi apollo chat --model qwen3.5-9b --precision Q4-affine-g64-bf16 --mode plain --max-output-tokens 64 --prompt 'Explain why the sky is blue.' --json
```

`Q4-affine-g64-bf16` means MLX affine four-bit weights, group size 64, bf16
activations. The exact model ID and any qualified `@apollo:X.Y.Z` route ID are
also accepted; the request is always sent as `@apollo:<installed version>`.

The binding is the app's code signature, not per-release file hashes: bundle id
`ai.sirsi.apollo`, signed with a Developer ID Application certificate of team
`9D382WV988` (`codesign --verify --strict --deep` against that requirement).
Every signed Apollo release is accepted without a Pantheon change; a modified,
re-signed or unsigned bundle refuses. The version comes from the signed bundle's
`CFBundleShortVersionString`. The model must be in this Mac's Stack Lab store
(`apollo-router checkout mlx-community/Qwen3.5-9B-4bit@8b2b98c0…`).

Modes are deliberately distinct:

| Command mode | Requested/required response tier | Meaning |
| --- | --- | --- |
| plain | plain | No drafter; literal autoregressive Plain |
| flash | flash-max | Apollo arithmetic; no MLX-equality claim |
| mlx-certified | mlx-certified | Separate speculative tier; not Plain |

There is no endpoint/profile/environment override or fallback. The signed app
must be at `~/Applications/Apollo.app` or `/Applications/Apollo.app` (the first
present is authoritative; a symlinked bundle refuses). A catalog entry proves an
installed signed binding, not that the model is resident, numerically certified,
or that inference succeeded. The signature check is not process attestation of
the loopback listener.

The existing installed Apollo service at 127.0.0.1:8520 owns admission, switching,
cancellation and idle teardown. The client does not start/restart/stop anything
or reserve/release native fleet fences. HTTP refusals, model/tier/drafter drift,
incomplete replies and transport failures fail without retry. HTTP proxy
environment variables and redirects cannot move a prompt off loopback.

This increment returns one bounded non-streamed text completion (1..4096 output
tokens; 32 KiB prompt; 4 MiB response; client timeout 120 seconds). It does not
execute tools, support native request/session IDs, or assert full 32768-position
context qualification. It never widens the native fleet's context/precision.
All non-Qwen models, including Gemma, retain the prior signed-native-profile
execution path. No native fleet profile, Gemma setting or service registry is
rewritten.

Source fixtures exercise typed request/response and refusal behavior without a
model or service. Real installed-command proof still requires normal admission,
signed Pantheon adoption and Rail's nonconflicting model window. Ra owns that
adoption; no competing v0.24.143 tag or direct installed-binary replacement is
part of this change. Rollback is a reviewed source revert and normal signed
client release; no Apollo service rollback is needed for this source-only patch.
