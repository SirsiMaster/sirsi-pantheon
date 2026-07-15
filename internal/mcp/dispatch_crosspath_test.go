package mcp

// PRD ROUTER_V2_DURABLE_DISPATCH /goal #3, proven: the CLI verbs and the MCP
// router_* handlers call ONE dispatch facade. A send through either path is
// visible through the other — same id, same file, same store row.

import (
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dispatch"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

// TestCrossPath_CLISendVisibleToMCP: a send through the facade exactly as the
// CLI's `sirsi router send` performs it is read back through the MCP handlers.
func TestCrossPath_CLISendVisibleToMCP(t *testing.T) {
	setupRouterRepoRoot(t)

	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	f, err := dispatch.Open(repoRoot) // the CLI's exact construction
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	res, err := f.Send("claude-pantheon", "codex-pantheon", "cli to mcp", "proposal", "one facade, one truth")
	if err != nil {
		t.Fatal(err)
	}

	pollRes, err := handleRouterPoll(map[string]interface{}{"agent": "codex-pantheon"})
	if err != nil {
		t.Fatal(err)
	}
	if text := resultText(t, pollRes); !strings.Contains(text, res.ID) {
		t.Fatalf("MCP poll does not see the CLI-path send %s:\n%s", res.ID, text)
	}
	getRes, err := handleRouterGet(map[string]interface{}{"id": res.ID})
	if err != nil {
		t.Fatal(err)
	}
	if text := resultText(t, getRes); !strings.Contains(text, "one facade, one truth") {
		t.Fatalf("MCP get does not serve the CLI-path item body:\n%s", text)
	}
}

// TestCrossPath_MCPSubmitVisibleToCLI: a send through the MCP handler is read
// back through the facade exactly as the CLI's `sirsi router pull` reads it.
func TestCrossPath_MCPSubmitVisibleToCLI(t *testing.T) {
	setupRouterRepoRoot(t)

	subRes, err := handleRouterSubmit(map[string]interface{}{
		"type":         "review",
		"author":       "codex-pantheon",
		"title":        "mcp to cli",
		"content":      "same facade backwards",
		"addressed_to": "claude-pantheon",
	})
	if err != nil || subRes.IsError {
		t.Fatalf("submit: %v / %s", err, resultText(t, subRes))
	}
	id := extractDocID(t, resultText(t, subRes))

	repoRoot, err := router.FindRepoRoot()
	if err != nil {
		t.Fatal(err)
	}
	f, err := dispatch.Open(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	items, err := f.Inbox("claude-pantheon") // the CLI pull's exact read
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != id {
		t.Fatalf("CLI pull does not see the MCP-path send %s: %+v", id, items)
	}
	text, err := f.Show(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "same facade backwards") {
		t.Fatalf("CLI show does not serve the MCP-path item body:\n%s", text)
	}
}
