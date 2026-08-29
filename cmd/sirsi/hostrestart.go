package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"

	"github.com/spf13/cobra"
)

var (
	hostRestartGOOS = runtime.GOOS
	hostRestartRun  = runHostRestartCommand
)

func runHostRestartCommand(name string, args ...string) error {
	command := exec.Command(name, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command.Run()
}

func newHostCommand() *cobra.Command {
	host := &cobra.Command{
		Use:   "host",
		Short: "Manage this Mac as a Pantheon host",
	}
	host.AddCommand(newHostRestartCommand(), newHostReachabilityCommand())
	return host
}

func newHostRestartCommand() *cobra.Command {
	var authenticated bool
	var confirmed bool
	var delayMinutes int

	command := &cobra.Command{
		Use:   "restart",
		Short: "Perform a planned FileVault-aware restart",
		Long: `Prepare one authenticated FileVault restart through Apple's fdesetup.

Pantheon does not store your password, disable FileVault, or enable automatic
login. Apple's tool prompts interactively, stages a one-restart unlock key,
restarts the Mac, and removes the key after successful startup. Pantheon and
SNE can then resume through their consented launchd registrations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if hostRestartGOOS != "darwin" {
				return fmt.Errorf("authenticated host restart is supported only on macOS")
			}
			if !authenticated {
				return fmt.Errorf("refusing an ordinary restart: pass --authenticated to preserve the post-FileVault login handoff")
			}
			if !confirmed {
				return fmt.Errorf("restart not confirmed: review the FileVault warning, then pass --confirm")
			}
			if delayMinutes < -1 {
				return fmt.Errorf("delay-minutes must be -1 or greater")
			}
			if err := hostRestartRun("/usr/bin/fdesetup", "isactive"); err != nil {
				return fmt.Errorf("FileVault is not active or its state could not be verified: %w", err)
			}
			if err := hostRestartRun("/usr/bin/fdesetup", "supportsauthrestart"); err != nil {
				return fmt.Errorf("this Mac does not report authenticated-restart support: %w", err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Pantheon is handing this one planned restart to Apple's authenticated FileVault restart. Your password is read only by sudo/fdesetup.")
			if err := hostRestartRun(
				"/usr/bin/sudo",
				"/usr/bin/fdesetup",
				"authrestart",
				"-delayminutes",
				strconv.Itoa(delayMinutes),
			); err != nil {
				return fmt.Errorf("authenticated restart was not scheduled: %w", err)
			}
			return nil
		},
	}

	command.Flags().BoolVar(&authenticated, "authenticated", false, "use Apple's one-restart FileVault unlock handoff")
	command.Flags().BoolVar(&confirmed, "confirm", false, "confirm the temporary FileVault protection reduction and restart")
	command.Flags().IntVar(&delayMinutes, "delay-minutes", 0, "minutes before restart; 0 is immediate and -1 prepares without restarting")
	return command
}

func init() {
	rootCmd.AddCommand(newHostCommand())
}
