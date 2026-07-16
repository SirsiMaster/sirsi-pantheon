package main

// launchdcmd.go — `sirsi launchd refresh`: re-derive launchd's pinned code
// requirements (LWCR) for every ai.sirsi.* job after a binary re-sign.
//
// Every deploy re-signs the sirsi binary (the AMFI cdhash cache demands it),
// and launchd keeps enforcing the OLD signature until each job is booted out
// and re-bootstrapped — kickstart does not clear a Launch Constraint
// Violation. Router item 20260716-150734: ai.sirsi.conduit.tick crash-looped
// 531 times post-re-sign until claude-home manually bootout+bootstrapped it.
// `make install` now runs this automatically; it is also safe to run by hand
// any time a sirsi job is crash-looping with "Code Signature Invalid".

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

var launchdCmd = &cobra.Command{
	Use:   "launchd",
	Short: "Manage the launchd jobs Sirsi installs (macOS)",
}

var launchdRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Bootout + re-bootstrap every ai.sirsi.* job — clears stale code-signature pins after a re-sign",
	Long: `After the sirsi binary is re-signed (every deploy), launchd keeps enforcing
the OLD signature on jobs that run it and SIGKILLs them in a crash-loop
("Launch Constraint Violation"). kickstart cannot clear it; only a bootout +
bootstrap re-derives the constraint from the binary as currently signed.

This refreshes every ~/Library/LaunchAgents/ai.sirsi.*.plist job. Run it after
any re-sign/deploy ('make install' does automatically), or whenever a sirsi
launchd job is crash-looping with "Code Signature Invalid".`,
	RunE: func(cmd *cobra.Command, args []string) error {
		refreshed, problems := setup.RefreshSirsiLaunchAgents()
		for _, label := range refreshed {
			fmt.Printf("  refreshed  %s\n", label)
		}
		for _, p := range problems {
			fmt.Printf("  problem    %s\n", p)
		}
		if len(refreshed) == 0 && len(problems) == 0 {
			fmt.Println("  no ai.sirsi.* launchd jobs found — nothing to refresh")
			return nil
		}
		if len(problems) > 0 {
			return fmt.Errorf("%d job(s) failed to re-bootstrap: %s",
				len(problems), strings.Join(problems, "; "))
		}
		fmt.Printf("✅ %d launchd job(s) re-bootstrapped — code-signature pins now match the installed binary\n", len(refreshed))
		return nil
	},
}

func init() {
	launchdCmd.AddCommand(launchdRefreshCmd)
}
