package main

// runner.go — `sirsi runner`: self-hosted CI on Sirsi silicon as a product
// verb (ADR-042 fleet-wide, ADR-044). `install` registers this Mac as a
// repo's build worker; `status` grades the whole local fleet against GitHub
// and feeds the board producer the same rows as JSON.

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/runner"
)

var runnerCmd = &cobra.Command{
	Use:   "runner",
	Short: "Builds on your own silicon — install and watch this Mac's CI runners",
}

var runnerInstallCmd = &cobra.Command{
	Use:   "install <owner>/<repo>",
	Short: "Register this Mac as a repository's build worker (reboot-durable service)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		owner, repo := runner.ParseRepoArg(args[0])
		return runner.Install(owner, repo, func(line string) {
			fmt.Println(line)
		})
	},
}

var runnerStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Every runner this Mac hosts, graded live against GitHub",
	RunE: func(cmd *cobra.Command, args []string) error {
		rows, err := runner.Status()
		if err != nil {
			return err
		}
		if JsonOutput {
			return json.NewEncoder(os.Stdout).Encode(rows)
		}
		if len(rows) == 0 {
			fmt.Println("No runners installed on this Mac yet — `sirsi runner install <repo>` sets one up.")
			return nil
		}
		for _, r := range rows {
			switch r.Status {
			case "online":
				state := "idle"
				if r.Busy {
					state = "working"
				}
				fmt.Printf("  ✓ %s — runner %s: online, %s\n", r.Repo, r.Name, state)
			case "unregistered":
				fmt.Printf("  ✗ %s — runner %s: installed here but GitHub no longer knows it (re-run install)\n", r.Repo, r.Name)
			case "unreachable":
				fmt.Printf("  ? %s — runner %s: couldn't reach GitHub to check\n", r.Repo, r.Name)
			default:
				fmt.Printf("  ✗ %s — runner %s: %s\n", r.Repo, r.Name, r.Status)
			}
		}
		return nil
	},
}

func init() {
	runnerCmd.AddCommand(runnerInstallCmd, runnerStatusCmd)
	rootCmd.AddCommand(runnerCmd)
}
