# Sirsi Local AI Stale Work Reconciliation

**Date:** 2026-08-22

## SNE v1 article reviews

Three open review requests targeted PR 281 and the retired SNE v1 public article.
The active goal now makes SNE v2 the sole product lineage and explicitly retires
SNE v1 from active claims. Reviewing or publishing the old v1 article would be
negative work: it would reintroduce the obsolete 59 tok/s package and old proxy
claims into the current public narrative. These requests are therefore closed as
coordination acknowledgements, superseded by the SNE v2 launch plan and the
zero-open-work charter. No approval of the old article is implied.

Superseded items:

- `20260815-150319-codex-inference-claude-deck-adversarial-review-sne-v1-sirsi-ai-article-pr-281`
- `20260815-150943-codex-inference-claude-home-fallback-adversarial-review-sne-v1-sirsi-ai-article-pr-281`
- `20260815-151250-codex-inference-codex-home-local-fallback-review-sne-v1-sirsi-ai-article-pr-281`

## Liveness-watch disabled-label decision

The liveness warning named ten experimental SNE labels. Their LaunchAgent files
were deliberately retired during owner-approved containment. Current Pantheon
diagnosis proves the override records are ignored only when the corresponding
plist is absent, reports containment explicitly, and scores `100/green`.
Re-enabling those labels would violate containment and revive experimental
workers. The decision request is closed as resolved by intentional retirement,
not by running `launchctl enable`.

Resolved item:

- `20260822-121431-horus-claude-pantheon-liveness-watch-ai-sirsi-runner-labels-disabled-in-launchd-ov`

## Live-state evidence

- Exactly one Sirsi LaunchAgent is loaded: `ai.sirsi.pantheon`.
- Exactly one resident Sirsi process is active outside the diagnostic command:
  `/Applications/Pantheon.app/Contents/MacOS/sirsi-menubar`.
- Swap is zero, no model broker is running, no runaway executor exists, and
  diagnosis is `100/green`.
- Liveness is reported as `intentionally contained`; retired override records do
  not create repair recommendations.
