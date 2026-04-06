# Osiris — Snapshot Keeper

Osiris detects uncommitted work, measures session drift, and warns before data loss. 5-level risk assessment with time-based escalation.

## Commands

### Full checkpoint assessment
```bash
pantheon osiris assess                           # Current directory
pantheon osiris assess /path/to/repo             # Specific repo
pantheon osiris assess --json                    # JSON output
```

Reports:
- Branch name and repo root
- File counts: uncommitted, staged, modified, untracked, deleted
- Diff stats: lines added/deleted
- Last commit hash, message, and time
- Risk level with warning if elevated

### Quick status
```bash
pantheon osiris status                           # One-line summary
pantheon osiris status --json                    # JSON for scripting
```

Returns a single line like `📄 11 files changed (last commit: 2h48m ago)` — suitable for menu bars, shell prompts, or CI scripts.

## Risk Levels

| Level | Trigger | Icon |
|-------|---------|------|
| None | Clean tree (0 files) | ✅ |
| Low | 1-5 files changed | 🟢 |
| Moderate | 6-15 files changed | 🟡 |
| High | 16-30 files changed | 🟠 |
| Critical | 30+ files OR 2+ hours since last commit | 🔴 |

At High and Critical levels, Osiris displays a warning recommending you commit.

## Use Cases

- **Pre-session check**: Run `osiris assess` before starting work to see what's pending
- **CI integration**: Use `osiris status --json` in pipelines to flag risky PRs
- **Menu bar**: Feed `osiris status` into a status bar widget for ambient awareness
- **Session end**: Check risk before closing your IDE to avoid losing uncommitted work
