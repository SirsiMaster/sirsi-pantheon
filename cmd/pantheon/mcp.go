package main

import (
	"fmt"
	"os"

	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the Pantheon Model Context Protocol (MCP) server",
	Run: func(cmd *cobra.Command, args []string) {
		// Ensure singleton for MCP server
		unlock, err := platform.TryLock("mcp-cli")
		if err != nil {
			fmt.Println("𓁟 Pantheon MCP Server is already active in another process.")
			return
		}
		defer unlock()

		fmt.Fprintln(os.Stderr, "𓁟 Pantheon MCP Server — Enabling persistent AI developer context...")

		// Create and run the MCP server (STDIO mode)
		server := mcp.NewServer()
		if err := server.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Error running MCP server: %v\n", err)
			os.Exit(1)
		}
	},
}
