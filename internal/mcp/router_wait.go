// Package mcp — router_wait.go
//
// Thin MCP wrappers over the file-based router (internal/work) plus a blocking
// long-poll, router_wait, that keeps an agent session non-idle while it waits
// for inbox work (anti-idle leg 2, Rule A27 — the loop IS the heartbeat).
//
// These tools reuse internal/work verbatim (Send/ListInbox/Get/Close/ListAll)
// — they do NOT reimplement or replace the file-based router. The items/ dir of
// markdown files remains the single source of truth.
package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/work"
	"github.com/fsnotify/fsnotify"
)

// maxWaitSeconds caps router_wait so a single long-poll can never pin a session
// past the MCP request budget. Callers above this are clamped, not rejected.
const maxWaitSeconds = 50

// workRoot resolves the repo's .agents/idea-router directory (read-only — does
// not create items/). Mirrors cmd/sirsi.workRoot so the MCP tools and the CLI
// observe the exact same queue.
func mcpWorkRoot() (string, error) {
	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		return "", fmt.Errorf("no .agents/idea-router/ found: %w", err)
	}
	return filepath.Join(repoRoot, ".agents", "idea-router"), nil
}

// ── Thin router wrappers ────────────────────────────────────────────────────

func handleRouterSend(args map[string]interface{}) (*ToolResult, error) {
	from, _ := args["from"].(string)
	to, _ := args["to"].(string)
	title, _ := args["title"].(string)
	msgType, _ := args["type"].(string)
	instructions, _ := args["instructions"].(string)

	if from == "" || to == "" || title == "" {
		return textResult("Error: from, to, and title are required.", true), nil
	}

	root, err := mcpWorkRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	if err = work.EnsureRoot(root); err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	id, err := work.SendTyped(root, from, to, title, msgType, instructions)
	if err != nil {
		return textResult(fmt.Sprintf("Error sending item: %v", err), true), nil
	}
	return textResult(fmt.Sprintf("Sent %s → %s: %s", from, to, id), false), nil
}

func handleRouterPull(args map[string]interface{}) (*ToolResult, error) {
	agent, _ := args["agent"].(string)
	if agent == "" {
		return textResult("Error: agent is required.", true), nil
	}

	root, err := mcpWorkRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	items, err := work.ListInbox(root, agent)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	return textResult(formatInbox(agent, items), false), nil
}

func handleRouterShow(args map[string]interface{}) (*ToolResult, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return textResult("Error: id is required.", true), nil
	}

	root, err := mcpWorkRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	it, err := work.Get(root, id)
	if err != nil {
		return textResult(fmt.Sprintf("Error reading item: %v", err), true), nil
	}
	return textResult(formatItem(it), false), nil
}

func handleRouterClose(args map[string]interface{}) (*ToolResult, error) {
	id, _ := args["id"].(string)
	if id == "" {
		return textResult("Error: id is required.", true), nil
	}
	result, _ := args["result"].(string)

	root, err := mcpWorkRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	if err = work.EnsureRoot(root); err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	if err = work.Close(root, id, result); err != nil {
		return textResult(fmt.Sprintf("Error closing item: %v", err), true), nil
	}
	return textResult(fmt.Sprintf("Closed %s", id), false), nil
}

func handleRouterStatus(_ map[string]interface{}) (*ToolResult, error) {
	root, err := mcpWorkRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	all, err := work.ListAll(root)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	var open, closed int
	perAgent := map[string]int{}
	for _, it := range all {
		if it.Status == "closed" {
			closed++
			continue
		}
		open++
		perAgent[it.To]++
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Router status: %d open, %d closed\n", open, closed))
	if open > 0 {
		sb.WriteString("\nOpen by recipient:\n")
		for agent, n := range perAgent {
			sb.WriteString(fmt.Sprintf("  %s: %d\n", agent, n))
		}
	}
	return textResult(sb.String(), false), nil
}

// handleRouterWait BLOCKS until an open item addressed to `agent` exists in the
// items/ dir, or timeout_s elapses (capped at maxWaitSeconds). It is the
// anti-idle long-poll: a session calls it on wake and stays parked — not idle —
// until real work arrives. Returns the matching item(s), or an empty result on
// timeout.
//
// Mechanism: fsnotify on the items/ directory. fsnotify is already a vendored
// direct dependency (go.mod) used by internal/horus/watcher.go, so this adds no
// new dependency. An initial inbox check covers items that already exist before
// the watcher arms (no missed-event race); the watcher then delivers any new
// item promptly. A timer bounds the wait.
func handleRouterWait(args map[string]interface{}) (*ToolResult, error) {
	agent, _ := args["agent"].(string)
	if agent == "" {
		return textResult("Error: agent is required.", true), nil
	}

	timeoutS := maxWaitSeconds
	if t, ok := args["timeout_s"].(float64); ok && t > 0 {
		timeoutS = int(t)
	}
	if timeoutS > maxWaitSeconds {
		timeoutS = maxWaitSeconds
	}

	root, err := mcpWorkRoot()
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutS)*time.Second)
	defer cancel()

	items, err := waitForInbox(ctx, root, agent)
	if err != nil {
		return textResult(fmt.Sprintf("Error: %v", err), true), nil
	}
	if len(items) == 0 {
		return textResult(fmt.Sprintf("No items for %s after %ds (timeout). Inbox is empty.", agent, timeoutS), false), nil
	}
	return textResult(formatInbox(agent, items), false), nil
}

// waitForInbox blocks until open items addressed to agent appear, or ctx is
// done. It checks the inbox first (covering pre-existing items and closing the
// watcher-arm race), then arms an fsnotify watch on items/ and re-checks on each
// filesystem event. A short fallback ticker re-polls so a missed/coalesced event
// never strands the wait.
func waitForInbox(ctx context.Context, root, agent string) ([]work.Item, error) {
	if items, err := work.ListInbox(root, agent); err != nil {
		return nil, err
	} else if len(items) > 0 {
		return items, nil
	}

	// Ensure items/ exists so the watcher has a directory to observe; a fresh
	// repo may not have materialized it yet.
	if err := work.EnsureRoot(root); err != nil {
		return nil, err
	}

	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create watcher: %w", err)
	}
	defer w.Close()

	if err := w.Add(filepath.Join(root, "items")); err != nil {
		return nil, fmt.Errorf("watch items dir: %w", err)
	}

	// Fallback re-poll guards against missed or coalesced fsnotify events.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, nil // timeout — empty, not an error
		case _, ok := <-w.Events:
			if !ok {
				return nil, nil
			}
			items, err := work.ListInbox(root, agent)
			if err != nil {
				return nil, err
			}
			if len(items) > 0 {
				return items, nil
			}
		case <-ticker.C:
			items, err := work.ListInbox(root, agent)
			if err != nil {
				return nil, err
			}
			if len(items) > 0 {
				return items, nil
			}
		case <-w.Errors:
			// Watcher hiccup — keep polling via the ticker rather than failing
			// the wait; the queue is still readable.
		}
	}
}

// ── Formatting helpers ──────────────────────────────────────────────────────

func formatInbox(agent string, items []work.Item) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Inbox for %s: %d open\n", agent, len(items)))
	if len(items) == 0 {
		sb.WriteString("\n  No open items.\n")
		return sb.String()
	}
	sb.WriteString("\n")
	for _, it := range items {
		typeLabel := it.Type
		if typeLabel == "" {
			typeLabel = "work"
		}
		sb.WriteString(fmt.Sprintf("  • %s\n      from: %s\n      type: %s\n      title: %s\n      opened: %s\n\n",
			it.ID, it.From, typeLabel, it.Title, it.Opened))
	}
	sb.WriteString("  Read full: router_show <id>. Close when done: router_close <id> result=...\n")
	return sb.String()
}

func formatItem(it work.Item) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("ID: %s\n", it.ID))
	sb.WriteString(fmt.Sprintf("From: %s\n", it.From))
	sb.WriteString(fmt.Sprintf("To: %s\n", it.To))
	if it.Type != "" {
		sb.WriteString(fmt.Sprintf("Type: %s\n", it.Type))
	}
	sb.WriteString(fmt.Sprintf("Status: %s\n", it.Status))
	sb.WriteString(fmt.Sprintf("Title: %s\n", it.Title))
	sb.WriteString(fmt.Sprintf("Opened: %s\n", it.Opened))
	if it.Closed != "" {
		sb.WriteString(fmt.Sprintf("Closed: %s\n", it.Closed))
	}
	sb.WriteString("\n## Instructions\n\n")
	sb.WriteString(it.Instructions)
	sb.WriteString("\n")
	if it.Result != "" {
		sb.WriteString("\n## Result\n\n")
		sb.WriteString(it.Result)
		sb.WriteString("\n")
	}
	return sb.String()
}
