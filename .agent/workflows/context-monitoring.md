---
description: How to monitor context window and token usage during development sessions
---

# Context & Token Monitoring Workflow (v2.1)

## Overview
Track session health using AG Monitor Pro (installed) plus agent-side heuristics.
Report after every sprint/phase to prevent context exhaustion.

## AG Monitor Pro (Installed ✅)
- **Location:** `~/.antigravity/extensions/shivangtanwar.ag-monitor-pro-1.0.0`
- **Source:** github.com/shivangtanwar/AG-Monitor-Pro (v1.0.0)  
- **How it works:** Watches `.pb` conversation logs in `~/.gemini/antigravity/conversations/`, extracts text, counts tokens via `cl100k_base`, applies adaptive I/O estimation curves
- **Dashboard:** Activity Bar icon → Webview with cost banner, I/O stats, usage chart, per-model breakdown
- **Persistence:** VS Code `globalState` — survives window reloads and restarts

### AG Monitor Pro Commands
| Command | Description |
|---------|-------------|
| `AG Monitor: Refresh Dashboard` | Force-refresh with latest data |
| `AG Monitor: Reset Session` | Clear all usage data |
| `AG Monitor: Export Usage Report` | Export as JSON (agent can read this) |

### Interaction Modes (Set to "Code Gen" for development)
| Mode | Best for | Input share |
|------|----------|-------------|
| Q&A | Short prompts, detailed answers | Lower (output-heavy) |
| **Code Gen** ← use this | Generating new code from prompts | Moderate |
| General | Mixed interactions | Neutral |
| Debug & Fix | Sharing errors, logs, traces | Higher (more input credit) |
| Review & Edit | Sending code/text for revision | Highest input share |

### How Agent Reads AG Monitor Data
1. User runs `AG Monitor: Export Usage Report` from command palette
2. Agent reads the exported JSON file
3. Agent incorporates real token data into session report

If export isn't available, agent falls back to calibrated heuristics (see below).

## Agent-Side Heuristics (Supplement)

When AG Monitor export isn't available, the agent tracks:

| Metric | How |
|--------|-----|
| Session wall-clock | Current time minus session start |
| Conversation depth | Count of user turns |
| Files ingested | Count of view_file/grep/list_dir calls with line counts |
| Output volume | Estimated lines of code/text generated |
| Commits | Count of git commits this session |
| Files modified | Count of distinct files changed |

### Heuristic Model
- **Fresh context (turns 1-5):** ~10-20% filled. Green zone.
- **Active development (turns 5-15):** ~20-60% filled. Peak productivity.
- **Deep context (turns 15-25):** ~60-85% filled. Watch for quality.
- **Critical (turns 25+):** >85% filled. Wrap protocol activated.

## Post-Sprint Report Format (MANDATORY)

```
## 📊 Session Metrics — Sprint [N]
| Metric | Value |
|--------|-------|
| ⏱️ Session elapsed | Xh Ym |
| 💬 Conversation depth | Turn N |
| 📂 Files ingested | N files (~XK lines) |
| ✏️ Output generated | ~N lines code/text |
| 🔀 Commits this session | N |
| 📝 Files modified | N |
| 💰 AG Monitor (if exported) | Input: X tokens, Output: Y tokens, Cost: $Z |

### Context Health
| Indicator | Status |
|-----------|--------|
| Estimated fill | ~XX% |
| Checkpoint signals | None / Detected |
| Degradation risk | Low / Medium / High |

### Recommendation
🟢 Continue | 🟡 Wrap within 2-3 tasks | 🔴 Wrap NOW
```

## Proactive Rules (Agent-Enforced)

1. After every sprint: Report metrics automatically
2. At 🟡: Announce "Getting deep — recommend wrapping within 2-3 more tasks"
3. At 🔴: STOP new work. Execute wrap protocol immediately.
4. Never fabricate precision — say "~60% estimated" not "62.4% used"
5. Challenge scope creep if context is 🟡 and user adds tasks

## Session Wrap Protocol (when 🟡 or 🔴)
1. Commit all work (Rule 29 format)
2. Push to GitHub
3. Update CHANGELOG.md
4. Generate CONTINUATION-PROMPT.md
5. Report final session metrics (with AG Monitor export if available)
