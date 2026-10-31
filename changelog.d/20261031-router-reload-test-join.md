### Fixed the invalid consumer reload lifecycle fixture

The wake-consumer reload regression now waits for the same controller thread's
capability and answer-role update after the old child exits. Its inert child
hold also demonstrates that the prior fixed-delay assertion can observe the
old grants before the required join and reload have happened.

Refs: `PANTHEON_RULES.md` A27, `docs/ADR-022-CTR-OS-TRUTH-LIVENESS.md`, `docs/ADR-070-MAAT-STRUCTURED-VERDICT.md`, `docs/stacklab/pantheon-pt/canon/TRACEABILITY_MATRIX.md`.
