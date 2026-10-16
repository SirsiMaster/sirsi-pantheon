# Ma’at launcher enforcement

Owner incident: Apollo51 ffd22719/subagent ad389 detached Python import, distinct from admitted Rail. Native SSA reports installed PreToolUse includes the Aug7 Go guard and display-power guard, without Ma’at. Native signing/install and the stdlib supervised runner are owned by SSA; this PR does not ship or exercise that runner.

## Command boundary

The tracked `.claude/hooks/maat-heavy-launch.py` reads a PreToolUse command without executing it. Only exact foreground `~/.local/bin/sirsi-maat-run` (or its actual user-home absolute expansion) may submit heavy execution. Bare-name/alternate-path front doors, env/shell wrappers, shell substitutions, line breaks, redirects, pipes, semicolon/background composition and `run_in_background:true` cannot use that exception. The runner must independently enforce actual registered runtime ancestry, host/agent/work, full-share exact live reservation, repeated expiry/id checks, bounded child-group cleanup and release of only its own acquired admission. Hook success alone is never admission.

Direct foreground/detached Python, Go build/test/run, unknown/model binaries and opaque shell wrappers are denied conservatively. Read-only allowlisted commands remain usable: e.g. `rg 'import mlx.core' source.py`, `git show HEAD:mlx.py`, `git diff -- mlx_lm.py`, `cat python-model.py | head -20`. Names in source-read arguments do not classify as execution. Executing read-tool features (`rg --pre`, git external diff/textconv) are refused. sed/find are not treated as read-only because their expressions can execute commands. Unknown commands fail closed: use the foreground front door; do not change permission mode, borrow admission, or move the same refused command into another session.

This is a conservative command boundary, not a shell sandbox or proof of complete classifier coverage. Local executable/config integrity, unusual aliases and tool adapters, unhooked clients and non-Bash execution paths remain native deployment limitations. Unknown/opaque commands are refused, not inferred safe.

## Native deployment and rollback

SSA installs exact reviewed hook at `~/.claude/hooks/maat-heavy-launch.py` and the independently tested runner at `~/.local/bin/sirsi-maat-run`. Merge `hooks/maat-launch.settings.json` into existing user PreToolUse preserving all other hooks and permissions; no blanket permission-mode change. Native settings.local hook list was reported null, not a second enforcement layer. Test actual attended/headless/subagent rejection before child spawn and foreground admission refusal/success with model-free actors, including permission-held cross-session fallback. Preserve existing settings backup and exact hashes for rollback. Do not enable the front door until runner tests pass. Removing this hook does not remove SSA’s runner admission checks, but removes this layer of coverage; record that fact explicitly.

Both legacy headless-worker scripts explicitly request bypassPermissions; the attended launcher supports AutoApprove. No such flags or live sessions are changed here. PreToolUse propagation and refusal under those actual modes must be measured on the native host. Existing original implementation request stays open until deployed controls pass. No observer signing or model process was performed by this source change.

References: ADR-006, ADR-061; AGENTS A17/A32.
