# Ra — Agent Orchestrator

Ra orchestrates all Pantheon deities across multiple repositories. He dispatches parallel AI agents to run health checks, tests, lints, and arbitrary tasks fleet-wide.

## Prerequisites

```bash
pip3 install claude-code-sdk     # Required for agent deployment
```

## Commands

### Health check
```bash
sirsi ra health               # Verify build, git, and commits across all repos
```

### Parallel testing
```bash
sirsi ra test                 # Run each repo's test suite in parallel
```

### Parallel linting
```bash
sirsi ra lint                 # Run linters across all repos
```

### Targeted task
```bash
sirsi ra task pantheon "fix the seba test failures"   # Task to specific repo
```

Dispatches a focused Claude agent to a single repo with full tool access.

### Fleet-wide broadcast
```bash
sirsi ra broadcast "check for security vulnerabilities"  # All repos
```

Runs the same prompt across every configured repository simultaneously.

### Nightly CI
```bash
sirsi ra nightly              # Three-phase: health → lint → test
```

### Multi-scope deployment
```bash
sirsi ra deploy               # Deploy scopes defined in configs/scopes/
sirsi ra deploy --scope auth  # Deploy specific scope
```

Scopes are YAML configs that define pre-approved task plans. Agents execute them autonomously without asking for confirmation.

### Monitor deployments
```bash
sirsi ra watch                # Live Command Center TUI
sirsi ra status               # Show orchestrator status
sirsi ra collect              # Collect results from completed agents
sirsi ra kill                 # Terminate all deployed windows
```

### Knowledge pipeline
```bash
sirsi ra health --record      # Record results through Seshat/Thoth pipeline
```

## Scope Configuration

Scopes live in `configs/scopes/*.yaml`. Each scope defines a directive task list that agents execute autonomously:

```yaml
name: fix-tests
repos: [pantheon]
tasks:
  - Run go test ./... and fix any failures
  - Commit with descriptive message
  - Push to main
```

See `configs/scopes/README.md` for the full authoring guide.

## Architecture

Ra spawns terminal windows, each running a Claude agent with `--dangerously-skip-permissions` (scopes are pre-approved). Agents report progress via stream-json output, and Ra monitors them through the Command Center TUI.
