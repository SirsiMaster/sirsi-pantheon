package main

// censuscmd.go — `sirsi thread census`: on-demand run of the Universal Thread
// Census (A32). The supervisor runs it every 10 minutes; this verb is the
// operator's immediate pass + visibility into what it decided.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var censusCmd = &cobra.Command{
	Use:   "census",
	Short: "Register every agent-class process (CPU + GPU surfaces) as a thread — no misses (A32)",
	RunE: func(cmd *cobra.Command, args []string) error {
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		routerRoot := filepath.Join(repoRoot, ".agents", "idea-router")
		procs, err := router.EnumerateCensusProcs()
		if err != nil {
			return fmt.Errorf("enumerate processes: %w", err)
		}
		actions := router.RunCensus(routerRoot, procs)
		if JsonOutput {
			return json.NewEncoder(os.Stdout).Encode(actions)
		}
		if len(actions) == 0 {
			fmt.Println("No agent-class processes found — nothing to census.")
			return nil
		}
		registered := 0
		for _, a := range actions {
			switch {
			case a.Error != "":
				fmt.Printf("  ✗ %s pid %d — %s\n", a.AgentID, a.Proc.PID, a.Error)
			case a.Outcome == router.CensusRegistered:
				registered++
				fmt.Printf("  + %s (%s) pid %d → %s\n", a.AgentID, a.Surface, a.Proc.PID, a.Thread)
			default:
				fmt.Printf("  ✓ %s (%s) pid %d — already tracked\n", a.AgentID, a.Surface, a.Proc.PID)
			}
		}
		fmt.Printf("Census: %d agent-class process(es), %d newly registered.\n", len(actions), registered)
		return nil
	},
}

func init() {
	threadCmd.AddCommand(censusCmd)
}
