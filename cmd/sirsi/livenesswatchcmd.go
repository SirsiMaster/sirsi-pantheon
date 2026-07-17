package main

// livenesswatchcmd.go — `sirsi liveness-watch`: the OS-level (launchd) liveness
// safety net (A32 durability item, routed by claude-home). `install` writes the
// LaunchAgent; `run` is what the LaunchAgent invokes each tick (gemma-probe +
// menubar-alive + memory-death, routing one non-dup blocker to `user`). It needs
// no Claude app and spawns no claude session — zero-leak by construction.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/liveness"
	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
)

var livenessWatchCmd = &cobra.Command{
	Use:   "liveness-watch",
	Short: "OS-level (launchd) reboot-proof watch: gemma broker, menubar, memory-death",
	Long: `A launchd LaunchAgent that watches the load-bearing surfaces WITHOUT the
Claude app — reboot-proof and zero-leak. Each tick it runs a real generation
probe against the gemma broker (health lies), checks the SwiftUI menubar is
alive, and reads the swap/free-RAM spiral, then routes ONE non-duplicate item to
the owner for the worst current blocker. It only sees and routes — it never kills
a process (A32/ADR-040 load-bearing safety).`,
}

var livenessRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the liveness checks once (invoked by the LaunchAgent each tick)",
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := workRoot()
		if err != nil {
			return err
		}
		return liveness.Run(root, os.Stdout)
	},
}

var livenessInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the launchd LaunchAgent (runs now + every 15 min, at every login)",
	RunE: func(cmd *cobra.Command, args []string) error {
		bin := setup.BinaryPath()
		repoRoot, err := router.FindRepoRoot()
		if err != nil {
			return fmt.Errorf("locate repo root: %w", err)
		}
		msg, err := liveness.Install(bin, repoRoot)
		if err != nil {
			return err
		}
		fmt.Println("  " + msg)
		return nil
	},
}

var livenessUninstallCmd = &cobra.Command{
	Use:   "uninstall",
	Short: "Remove the launchd LaunchAgent",
	RunE: func(cmd *cobra.Command, args []string) error {
		msg, err := liveness.Uninstall()
		if err != nil {
			return err
		}
		fmt.Println("  " + msg)
		return nil
	},
}

var livenessStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report whether the LaunchAgent is installed",
	RunE: func(cmd *cobra.Command, args []string) error {
		if liveness.Installed() {
			fmt.Printf("  installed: %s\n", liveness.PlistPath())
			if fi, err := os.Stat(filepath.Join("/tmp", "sirsi-liveness-watch.log")); err == nil {
				fmt.Printf("  last ran:  %s\n", fi.ModTime().Format("2006-01-02 15:04:05"))
			}
			return nil
		}
		fmt.Println("  not installed — run `sirsi liveness-watch install`")
		return nil
	},
}

func init() {
	livenessWatchCmd.AddCommand(livenessRunCmd, livenessInstallCmd, livenessUninstallCmd, livenessStatusCmd)
	rootCmd.AddCommand(livenessWatchCmd)
}
