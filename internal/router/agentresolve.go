package router

import (
	"os"
	"strings"
	"time"
)

// ResolveCurrentAgent identifies the acting agent for a router write (send,
// close, respond, etc.), in the same order the CLI and MCP surfaces must both
// honor so a write's attribution never depends on which surface issued it:
// explicit override, then $SIRSI_AGENT_ID, then a session marker, then — only
// when unambiguous — the sole live thread registered on this host. Returns
// the resolved agent id and, on failure, a reason string explaining why not.
func ResolveCurrentAgent(routerRoot, override string) (string, string) {
	if a := strings.TrimSpace(override); a != "" {
		return a, "flag"
	}
	if a := strings.TrimSpace(os.Getenv("SIRSI_AGENT_ID")); a != "" {
		return a, "env SIRSI_AGENT_ID"
	}
	if a := ReadSessionAgentMarker(CurrentSessionID()); a != "" {
		return a, "session marker"
	}
	if reg, err := LoadThreadRegistry(routerRoot); err == nil {
		var candidates []string
		seen := map[string]bool{}
		now := time.Now().UTC()
		for _, t := range reg.SortedThreads() {
			if t.Status.IsTerminal() {
				continue
			}
			if EffectiveStale(t, now, DefaultThreadStaleAfter) {
				continue
			}
			if !seen[t.AgentID] {
				seen[t.AgentID] = true
				candidates = append(candidates, t.AgentID)
			}
		}
		if len(candidates) == 1 {
			return candidates[0], "sole live thread"
		}
	}
	return "", "could not resolve the current agent — pass --agent <id> (no $SIRSI_AGENT_ID, no session marker, and not a sole live thread)"
}
