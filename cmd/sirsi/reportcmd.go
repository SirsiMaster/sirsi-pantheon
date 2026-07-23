package main

// reportcmd.go — `sirsi report`: what the fabric DID, owner-readable (local
// sovereignty, owner directive 2026-07-23). Renders the run reports the
// supervisor (and the cloud conduit) write to ~/.sirsi/conduit-report.json —
// heals, escalations, and whether the cloud API was reachable — so a reboot
// recovery or a broker restore reaches the OWNER, not just an agent journal.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/report"
)

var reportLimit int

var reportCmd = &cobra.Command{
	Use:   "report",
	Short: "What Pantheon did on its own — recent self-checks, heals, and escalations",
	RunE: func(cmd *cobra.Command, args []string) error {
		f, err := report.Load(report.Path())
		if err != nil {
			return err
		}
		if JsonOutput {
			return json.NewEncoder(os.Stdout).Encode(f)
		}
		if len(f.Runs) == 0 {
			fmt.Println("No self-check reports yet — the supervisor writes one each pass.")
			return nil
		}
		n := reportLimit
		if n <= 0 || n > len(f.Runs) {
			n = len(f.Runs)
		}
		for _, r := range f.Runs[:n] {
			fmt.Println("  " + report.Sentence(r))
		}
		return nil
	},
}

func init() {
	reportCmd.Flags().IntVar(&reportLimit, "limit", 5, "How many recent runs to show")
	rootCmd.AddCommand(reportCmd)
}
