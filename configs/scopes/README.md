# Ra Scope Configs

Each YAML file defines one autonomous agent scope for `sirsi ra deploy`.

## Schema

```yaml
name: scope-id            # kebab-case, matches filename
display_name: "Human Name"
repo_path: "~/Development/RepoName"
deadline: "2026-04-15"    # ISO date or empty
priority: "P0"            # P0 (critical) through P3 (backlog)
max_turns: 50             # agent turn limit
scope_of_work: ""         # empty = dynamic (Neith derives from canon)
```

## Dynamic vs Static Scopes

### Dynamic (default — `scope_of_work: ""`)

When `scope_of_work` is empty, Neith reads the repo's full canon and the agent
derives its own scope from the continuation prompt, planning docs, blueprints,
ADRs, and Thoth memory. The agent checks `git log` to see what's done, identifies
the next incomplete phase/sprint, and executes it.

This is the correct mode for ongoing development. The canon IS the plan.

### Static (override — `scope_of_work: "1. Do X\n2. Do Y"`)

When `scope_of_work` has content, it's used as-is. The agent executes the
numbered tasks in order. Use this for one-off tasks like "fix CI" or
"deploy to production" that aren't part of the repo's canonical plan.

## How Neith Weaves Prompts

Neith assembles the final prompt in priority order:

1. **Ra Autonomy Directive** (injected by code) — overrides CLAUDE.md Rule 14
2. **Scope of Work** — static from YAML, or dynamic instructions
3. **Continuation Prompt** — `docs/CONTINUATION-PROMPT.md` (current state, next phases)
4. **Planning Docs** — full text of `*BLUEPRINT*`, `*PLAN*`, `*SCOPE*`, `*ROADMAP*`, `*SPECIFICATION*`, `*STATUS*`, `*BUILD_LOG*` files from `docs/`
5. **Thoth Memory + Journal** — `.thoth/memory.yaml` and `.thoth/journal.md`
6. **ADR Summaries** — first 10 lines of each `docs/ADR-*.md`
7. **Project Identity** — CLAUDE.md (truncated to 2000 chars; agent reads full file)
8. **Version + Changelog**

Content is truncated from the bottom (lowest priority) when hitting the 100K char token budget.
The directive and scope are always at the top and never truncated.

## Why stream-json (not default --print)

`claude --print` in default text mode buffers ALL output until the entire session completes.
For multi-step scopes with heavy tool use, this means 10+ minutes of zero output — agents
appear lifeless even though they're working.

Ra uses `--output-format stream-json --verbose` to get real-time JSON events, then pipes
through a python filter that extracts human-readable text and `[tool: Name]` summaries.
This gives live progress in the terminal window AND captures a log file for `sirsi ra collect`.

Do NOT change this back to default `--print` text mode. See Rule A24 in CLAUDE.md.

## Testing a Scope

```bash
sirsi ra deploy --dry-run --scope your-scope
```

Verify:
- The autonomy directive appears at the top
- The continuation prompt is present and not truncated
- Planning docs are included (check with `grep "^## " prompt.md`)
