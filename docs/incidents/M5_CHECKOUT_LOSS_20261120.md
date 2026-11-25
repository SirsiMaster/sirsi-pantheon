# M5 checkout loss, 2026-11-20 — record

**Recorded:** 2026-11-25 by Ra. Task `m5-pantheon-checkout-restore`.

**What was found.** On the M5, `/Users/thekryptodragon/Development/sirsi-pantheon` (branch `codex/pantheon-release-baseline-0.24.153-20261031`, 39383ba0) had 2,607 of 2,947 tracked files missing. SSA reported it on 2026-11-22. The Git index was last written on 2026-10-31 and the checkout is not sparse, so Git did not remove them. The folder was last changed at 2026-11-19 23:38:24 CST (2026-11-20 05:38Z).

**Likely cause.** `claude-finalwishes-m5` reported on 2026-11-25 that at about 05:36Z that day a sub-agent ran a deliberately weakened delete helper, to prove a cleanup test fails without its fix. The home folder was among its targets and it ran for about two minutes. It was not intended. No single command is tied to these files; the match is by time and by the neighboring `~/Development/.agents` loss that lane recorded.

**Restore.** The owner approved it on 2026-11-25. Missing tracked files were recreated with `git checkout-index` without force, which does not overwrite. Result: 0 missing, tracked status clean, all 1,625 files listed as untracked beforehand still present. The lists are on the M5 in `~/.sirsi/evidence/m5-checkout-restore-20261125/`.

**Not covered.** That lane also recorded losses in `~/.config` (gh, gcloud and Firebase sign-ins), the cached Go toolchains and `~/Development/.agents`. Ra has not checked or restored those.

**Rule.** Destructive test code, including a deliberately broken control, runs only inside a fresh temporary directory and asserts its target is inside it first. It never receives a home folder, `/` or a repository root.
