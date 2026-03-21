package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/mcp"
	"github.com/SirsiMaster/sirsi-anubis/internal/output"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "🔌 Start MCP server for AI IDE integration",
	Long: `🔌 MCP Server — Context Sanitizer for AI

Starts a Model Context Protocol (MCP) server over stdio.
AI coding assistants (Claude, Cursor, Windsurf, Codex) can connect
to Anubis as a context sanitizer for workspace hygiene.

MCP Tools:
  scan_workspace    Scan a directory for infrastructure waste
  ghost_report      Hunt for remnants of uninstalled apps
  health_check      Quick system health summary
  classify_files    Classify files semantically (junk, project, etc)

MCP Resources:
  anubis://health-status    System health as JSON
  anubis://capabilities     Available modules and commands
  anubis://brain-status     Neural brain installation status

Configuration for Claude Code (~/.claude/claude_desktop_config.json):
  {
    "mcpServers": {
      "anubis": {
        "command": "anubis",
        "args": ["mcp"]
      }
    }
  }

Configuration for Cursor/Windsurf (.cursor/mcp.json):
  {
    "mcpServers": {
      "anubis": {
        "command": "anubis",
        "args": ["mcp"]
      }
    }
  }

All scanning is local — no data leaves this machine (Rule A11).`,
	Run: runMCP,
}

func runMCP(cmd *cobra.Command, args []string) {
	// In MCP mode, all output goes to stderr so stdout is clean for JSON-RPC
	if !quietMode {
		fmt.Fprintln(os.Stderr, "𓂀 Anubis MCP Server")
		fmt.Fprintln(os.Stderr, "  Protocol: JSON-RPC 2.0 over stdio")
		fmt.Fprintln(os.Stderr, "  Tools: scan_workspace, ghost_report, health_check, classify_files")
		fmt.Fprintln(os.Stderr, "  Resources: health-status, capabilities, brain-status")
		fmt.Fprintln(os.Stderr, "  Waiting for client connection...")
		fmt.Fprintln(os.Stderr)
	}

	server := mcp.NewServer()
	if err := server.Run(); err != nil {
		output.Error("MCP server error: %v", err)
	}
}
