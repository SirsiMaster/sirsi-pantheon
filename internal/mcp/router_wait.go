package mcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// router_wait is the BLOCKING counterpart to router_poll. It parks the caller
// until inbox work addressed to `agent` arrives, or until `timeout_s` elapses
// (capped at 50s to stay under the MCP tool-call ceiling). This is the anti-idle
// primitive: instead of ending a turn and going idle on an empty inbox, a
// session calls router_wait to stay active until the next item lands.

const routerWaitMaxTimeout = 50

// waitForInbox polls the agent's inbox roughly once per second until it is
// non-empty or the deadline passes. Returns the pending item IDs, which may be
// empty if the deadline arrived first. Files remain the source of truth; this is
// a thin blocking wrapper over the same PollInbox that router_poll uses.
func waitForInbox(r *router.Router, agent string, deadline time.Time) ([]string, error) {
	for {
		pending, err := r.PollInbox(agent)
		if err != nil {
			return nil, err
		}
		if len(pending) > 0 {
			return pending, nil
		}
		if !time.Now().Before(deadline) {
			return nil, nil
		}
		time.Sleep(time.Second)
	}
}

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
	r, err := router.New(repoRoot)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	pending, err := waitForInbox(r, agent, time.Now().Add(time.Duration(timeout)*time.Second))
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	if len(pending) == 0 {
		return textResult(fmt.Sprintf("router_wait: no inbox items for %s after %ds. Inbox is clear; re-call to keep waiting.", agent, timeout), false), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("router_wait: %d item(s) arrived for %s\n\n", len(pending), agent))
	for _, id := range pending {
		doc, getErr := r.Get(id)
		if getErr != nil {
			sb.WriteString(fmt.Sprintf("  [?] %s (could not load: %v)\n", id, getErr))
			continue
		}
		sb.WriteString(fmt.Sprintf("  [%s] %s — %s (by %s)\n", doc.Type, doc.ID, doc.Title, doc.Author))
	}
	sb.WriteString("\nUse router_get to read, or router_poll with ack=true to clear.\n")
	return textResult(sb.String(), false), nil
}
