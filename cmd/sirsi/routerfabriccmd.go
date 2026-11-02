package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/SirsiMaster/sirsi-pantheon/internal/ledger"
	"github.com/spf13/cobra"
)

// Native callers use the protected bundled CLI, not a PATH lookup or a second
// service. This is the same read/join as GET /api/fabric. One-shot activity is
// explicitly unseeded: no invented persistent transition history.
func newRouterFabricCommand(read func(context.Context) (ledger.FabricBoard, error)) *cobra.Command {
	// Keep the machine-output contract even when this command is executed
	// standalone: Cobra otherwise appends usage to stdout after a read refusal.
	return &cobra.Command{Use: "fabric", SilenceUsage: true, Short: "Read the unified task/message/runtime/host FabricBoard", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		value, err := read(cmd.Context())
		if err != nil {
			return fmt.Errorf("fabric unavailable: %w", err)
		}
		if err := cmd.Context().Err(); err != nil {
			return fmt.Errorf("fabric unavailable: %w", err)
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(value)
	}}
}
func init() { routerCmd.AddCommand(newRouterFabricCommand(collectDashboardFabric)) }
