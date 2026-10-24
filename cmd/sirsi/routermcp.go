package main

import (
	"context"
	"io"
	"os/signal"
	"syscall"

	"github.com/SirsiMaster/sirsi-pantheon/internal/routermcp"
	"github.com/spf13/cobra"
)

func newRouterMCPCmd(run func(context.Context, io.ReadCloser, io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use: "router-mcp", Short: "Serve the bare router A2A MCP interface over stdio",
		Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		// No general CLI logging/FDA startup on the protocol stdout path.
		PersistentPreRun: func(*cobra.Command, []string) {},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			in, ok := cmd.InOrStdin().(io.ReadCloser)
			if !ok {
				in = io.NopCloser(cmd.InOrStdin())
			}
			return run(ctx, in, cmd.OutOrStdout())
		},
	}
}

func init() { rootCmd.AddCommand(newRouterMCPCmd(routermcp.Run)) }
