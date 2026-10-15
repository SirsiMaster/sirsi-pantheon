## Router Addressing Law

Every router item must be addressed to exactly one repo-scoped agent unless a written super-agent mandate exists.

Use this addressing formula:

```text
<agent-family>-<repo-or-workstream>
```

Examples:

- FinalWishes repo review for Claude: `claude-finalwishes`
- FinalWishes repo review for Codex: `codex-finalwishes`
- Pantheon router/CLI work for Codex: `codex-pantheon`
- Sirsi Nexus work for Codex: `codex-nexus`
- Assiduous work for Claude: `claude-assiduous`

Do not address FinalWishes work to `codex-pantheon` just because the router lives in Pantheon. Pantheon is the router home; the target repo still determines the agent id.

### Choosing The Target Agent

1. Identify the repo that owns the implementation or review.
2. Pick the agent family requested or implied by the work: `codex`, `claude`, `gemini`, `gemma`, `qwen`, or another registered family.
3. Look up the exact id in `/Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router/agents.json`.
4. Put the item under `pending.<agent_id>` only.
5. Use `pending_for_codex` or `pending_for_claude` only for legacy compatibility when no repo-scoped registered id exists. If you must use a legacy field, it must contain plain string document ids only and must not create a duplicate route to the wrong repo agent.

### State JSON Shape

`state.json` pending queues are machine-readable and must remain arrays of strings:

```json
{
  "pending": {
    "codex-finalwishes": ["20260520-example-doc-id"],
    "codex-pantheon": []
  },
  "pending_for_codex": [],
  "pending_for_claude": []
}
```

Never put metadata objects inside `pending`, `pending_for_codex`, or `pending_for_claude`. Metadata belongs in the proposal/review/decision frontmatter and body.

Invalid:

```json
{
  "pending": {
    "codex-finalwishes": [
      {
        "id": "20260520-example-doc-id",
        "eta_for_review": "2026-05-20T22:00:00-04:00"
      }
    ]
  }
}
```

That object-valued form breaks the Go router parser and stalls automation.

### Required Artifact Frontmatter

Each routed artifact should include:

```yaml
id: 20260520-agent-repo-topic
author: claude-finalwishes
addressed_to: codex-finalwishes
topic: finalwishes-tier1-ga
repo: /Users/thekryptodragon/Development/FinalWishes
agent_scope: repo-segmented
eta_for_review: 2026-05-20T22:00:00-04:00
next_check_at: 2026-05-20T22:00:00-04:00
estimated_duration: 1 hour
```

### Super-Agent Exception

A broad coordinator may route or edit across repos only when a router artifact explicitly names it as a super agent and lists:

- repositories in scope
- whether it may edit or only coordinate
- repo-scoped implementation owners
- verification evidence required before `/goal` completion

Without that mandate, route work to the repo owner agent and stop there.

# Router Addressing Prompt Snippet

Use this block when starting a Claude, Codex, Gemini, Gemma, Qwen, or other agent that needs to write to CTR.

```text
Before work, read /Users/thekryptodragon/Development/AGENTS.md and the target repo AGENTS.md.

Use the Ra Idea Router at /Users/thekryptodragon/Development/sirsi-pantheon/.agents/idea-router.

You are repo-scoped. Determine your agent_id as <agent-family>-<repo-or-workstream> from agents.json. Do not work across repos unless a written super-agent mandate explicitly grants that scope.

When routing work:
- write the proposal/review/decision markdown first
- set frontmatter id, author, addressed_to, topic, repo, agent_scope, eta_for_review, next_check_at, estimated_duration
- add only the document id string to state.json pending.<addressed_to>
- do not put objects in pending arrays
- do not use pending_for_codex or pending_for_claude if a repo-scoped registered target exists
- keep working until /goal is met, blocked with evidence, or impossible with a concrete reason

Examples:
- FinalWishes to Claude: pending.claude-finalwishes = ["<doc-id>"]
- FinalWishes to Codex: pending.codex-finalwishes = ["<doc-id>"]
- Pantheon router work to Codex: pending.codex-pantheon = ["<doc-id>"]
```
