package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// router_wait is the BLOCKING counterpart to router_poll. It parks the caller
// until inbox work addressed to `agent` arrives, or until `timeout_s` elapses
// (capped at 50s to stay under the MCP tool-call ceiling). This is the anti-idle
// primitive: instead of ending a turn and going idle on an empty inbox, a
// session calls router_wait to stay active until the next item lands.
//
// Router v2 Phase 3: this is now a REAL blocking wait over the dispatch
// facade — an item sent through the facade wakes the waiter event-driven in
// well under 250ms (PRD /goal #1); legacy file-only writers are caught by the
// facade's bounded 5s re-check. The 1-second poll loop this replaces was the
// PRD's "honest poll loop in a long-poll costume".

const routerWaitMaxTimeout = 50

func handleRouterWait(args map[string]interface{}) (*ToolResult, error) {
	agent, _ := args["agent"].(string)
	if strings.TrimSpace(agent) == "" {
		return textResult("router_wait requires 'agent' (your agent name).", true), nil
	}

	timeout := routerWaitMaxTimeout
	if t, ok := args["timeout_s"].(float64); ok && t > 0 {
		timeout = int(t)
	}
	if timeout > routerWaitMaxTimeout {
		timeout = routerWaitMaxTimeout
	}

	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	defer func() { _ = f.Close() }()

	items, err := f.Wait(context.Background(), agent, time.Duration(timeout)*time.Second)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	if len(items) == 0 {
		return textResult(fmt.Sprintf("router_wait: no inbox items for %s after %ds. Inbox is clear; re-call to keep waiting.", agent, timeout), false), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("router_wait: %d item(s) waiting for %s\n\n", len(items), agent))
	for _, it := range items {
		kind := it.Type
		if kind == "" {
			kind = "item"
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s — %s (from %s)\n", kind, it.ID, it.Title, it.From))
	}
	sb.WriteString("\nUse router_get to read; close with `sirsi router close <id> --result …` when done.\n")
	return textResult(sb.String(), false), nil
}
