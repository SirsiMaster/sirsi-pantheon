# Owner decisions of 2026-11-04 — record

**Recorded:** 2026-11-11 by Ra. **Source:** the owner's answers in the Ra session of 2026-11-04, as written on each task's ledger row that day, and the owner's instruction of 2026-11-11 that Ra withdraw these six tasks with the decision recorded.

Six tasks on Ra's ledger were opened to get an owner decision. The owner gave each decision on 2026-11-04. The tasks then sat as `blocked` only because closing them was reserved to the owner. This file is the durable record of those decisions, so the six tasks can be withdrawn in its favour. The carrying task is `ra/owner-decisions-20261104-record`.

Nothing here authorises new work. Each entry says what was decided and what would reopen it.

| Task | Question | Decision | Reopen only if |
|---|---|---|---|
| `signing-escrow-loose-keys-m5` | Escrow, then delete, the loose signing-key copies on the M5. | Already done. The signing canon (section 6) records the loose copies deleted on 2026-09-29. Ra confirmed on 2026-11-04 that the two named M5 paths no longer exist. Ra did not read any key. | A loose copy is found again. |
| `signing-p12-password-off-appleid` | Is the escrowed `Certificates.p12` password stored separately from the Apple ID? | **Yes.** The owner confirmed it on 2026-11-04. | The storage arrangement changes. |
| `desktop-recovery-admission-issuer-owner-gate` | Should a Ra-owned operator-admission signing authority exist for `pantheon.desktop-recovery`? | **Not now.** No such authority is created. Ra does not generate or read private keys. Lanes must not re-ask. | The owner asks for desktop recovery to be built. |
| `bind-identity-missing-on-m1` | Provision the `sirsi-bind` key on the M1, or route binds through the host that has it? | **Keep the bind key off the M1.** Formal approvals go through the host that holds the key. | The key-holding host is retired. |
| `codex-github-review-publication-blocked-20261015` | Fix the connector and token so `codex-pantheon` can publish formal GitHub reviews? | **Leave as is.** Decided by Ra on 2026-11-04 under its router remit, after the owner had been asked twice. Nothing waits on it: the eight named pull requests are closed, and Codex reviews travel through the router. | A pull request actually needs the formal GitHub review mark. |
| `apollo-admission-authority-decision-20261015` | Should Pantheon gatekeep Apollo's model-execution and GPU resources, or stay observe-only? | **Observe-only.** Pantheon does not gatekeep Apollo GPUs. | The owner asks for active admission. |

## Notes

- The claim recorded on the Apollo task, that an admission adapter was "sealed" in commit `259e2d6e`, was checked and is false: that commit and file exist nowhere in this repository's history. The decision above does not depend on it.
- Withdrawn is not done. These tasks never count as completed work; they are retired because the question each one asked has an answer.
