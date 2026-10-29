# Installed Apollo 2.2.6 Qwen bridge

Platform-foundation source increment, not an installed-client or runtime certification.

The normal command selects the exact installed Qwen route with explicit precision
and mode. After normal reviewed, tested and signed Pantheon adoption:

```sh
sirsi apollo catalog --json
sirsi apollo chat --model qwen3.5-9b --precision Q4-affine-g64-bf16 --mode plain --max-output-tokens 64 --prompt 'Explain why the sky is blue.' --json
```

`Q4-affine-g64-bf16` means MLX affine four-bit weights, group size 64, bf16
activations. The exact model ID and the qualified `@apollo:2.2.0` route ID are
also accepted. This route version identifies the engine contract, not the app's
release version. The installed app binding is **2.2.6**.

Modes are deliberately distinct:

| Command mode | Requested/required response tier | Meaning |
| --- | --- | --- |
| plain | plain | No drafter; literal autoregressive Plain |
| flash | flash-max | Apollo arithmetic; no MLX-equality claim |
| mlx-certified | mlx-certified | Separate speculative tier; not Plain |

There is no endpoint/profile/environment override or fallback. Five exact app
artifact hashes must match the reviewed release at `~/Applications/Apollo.app`
or `/Applications/Apollo.app`. A catalog entry proves an installed binding, not
that the model is resident, numerically certified, or that inference succeeded.
Missing/changed artifacts refuse execution; a newer Apollo release needs a new
reviewed binding. This byte check is not a replacement for normal signed package
adoption, nor independent process/signature attestation of a loopback listener.

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
