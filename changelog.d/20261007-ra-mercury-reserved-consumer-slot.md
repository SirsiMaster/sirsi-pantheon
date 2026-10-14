### Changed

- **`mercury` shares the reserved consumer slot.** Same mechanism as `claude-pantheon` (#1028): the host consumer cap (2 on a 10-core Mac) was starving mercury behind long-running FinalWishes consumers, with no bonus-slot eligibility of its own. Immediate relief only — the shared single bonus slot among all `reserved_slot` lanes is still a flat allowlist, not general age-based fairness; tracked on the `ra` ledger (`consumer-slots-starve-reviewers`) as the real follow-up.
