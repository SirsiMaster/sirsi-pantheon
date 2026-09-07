package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/routerboard"
	"github.com/spf13/cobra"
)

var routerControlCmd = &cobra.Command{
	Use:   "control",
	Short: "Inspect the canonical worker control plane as one JSON envelope",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		bin, err := os.Executable()
		if err != nil || bin == "" {
			return fmt.Errorf("resolve canonical sirsi executable: %w", err)
		}
		agentsJSON := filepath.Join(repoRoot, ".agents", "idea-router", "agents.json")
		board := routerboard.New(bin, agentsJSON, routerboard.ControlSchema)
		board.Poll(context.Background())
		body, version, err := board.SnapshotControl()
		if err != nil {
			return fmt.Errorf("build control snapshot: %w", err)
		}
		if version == 0 || len(body) == 0 {
			return fmt.Errorf("control snapshot unavailable: canonical router poll did not complete")
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal(body, &out); err != nil {
			return fmt.Errorf("validate control snapshot: %w", err)
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	},
}

func init() { routerCmd.AddCommand(routerControlCmd) }
