# Ra Scope Configs

Each YAML file defines one autonomous agent scope for `pantheon ra deploy`.

## Schema

```yaml
name: scope-id            # kebab-case, matches filename
display_name: "Human Name"
repo_path: "~/Development/RepoName"
deadline: "2026-04-15"    # ISO date or empty
priority: "P0"            # P0 (critical) through P3 (backlog)
max_turns: 50             # agent turn limit
scope_of_work: |
  ...
```

## Scope Authoring Rules

Ra agents run in `claude --print` mode (non-interactive, no human on the other end).
If an agent asks a question, it hangs forever. Write scopes that eliminate questions.

### 1. Be directive, not descriptive

Bad: `CoreML classifier -- CGO bridge in internal/brain/inference.go`
Good: `internal/brain/inference.go -- extend the existing CGO bridge to call CoreML's MLModel.prediction(from:) via C interop. Add build tags for darwin/arm64.`

### 2. Start with "Read X first"

Every scope should begin with: `Read CLAUDE.md and the <dir> structure first, then execute in order:`

This grounds the agent in the repo's current state before it starts making changes.

### 3. Reference existing files

If the work extends existing code, name the files. If the work creates new code, name the target path.

Bad: `Add tests for the API`
Good: `Add tests for all Go API endpoints in cmd/api/ -- use httptest.NewServer pattern`

### 4. Number the tasks

Agents execute numbered tasks in order. Unnumbered bullets invite the agent to prioritize (and ask you about it).

### 5. Include a skip-and-continue rule

If a task might be blocked (missing dep, broken upstream), say what to do:
`If CoreML headers are missing, create a minimal Objective-C bridge file.`

### 6. End with commit+thoth

Every scope should end with:
```
N. Commit, push, run pantheon thoth compact.
```

### 7. Defer what doesn't fit

Don't pack 12 tasks into one scope. If something is lower priority, say:
`Skip X for now (deferred to next sprint).`

## How Neith Weaves Prompts

Neith assembles the final prompt sent to each agent:

1. **Ra Autonomy Directive** (injected by code, not editable) -- overrides CLAUDE.md Rule 14 (sprint plan approval). Tells the agent to execute without asking.
2. **Your Scope of Work** -- the `scope_of_work` field from this YAML file.
3. **Canon Context** (truncated to fit 32K token budget):
   - CLAUDE.md (first 2000 chars)
   - Thoth memory + journal
   - Continuation prompt
   - ADR summaries
   - Version + changelog

The directive and scope are **never truncated**. Canon context is expendable.

## Testing a Scope

```bash
pantheon ra deploy --dry-run --scope your-scope
```

This shows the assembled prompt without spawning windows. Verify:
- The autonomy directive appears at the top
- Your scope of work is fully visible (not truncated)
- The canon context provides enough project identity
