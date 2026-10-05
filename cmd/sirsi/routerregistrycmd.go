package main

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
)

var routerRegistryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Pin this host's agent registry to origin/main (A37) instead of a shared working tree",
	Long: `The registry (agents.json) was read from the working tree of a shared checkout, so
whatever branch or uncommitted edit another session left there became the fabric's
identity (a lane flipping to wake=none and reading WATCH_ONLY, "identity is not fully
declared"). After 'registry sync' this host reads a snapshot of origin/main; a
working-tree edit cannot change who the lanes are, and writes are refused until the
change is merged. 'registry unpin' goes back to the working tree.`,
}

var routerRegistrySyncCmd = &cobra.Command{
	Use: "sync", Short: "Fetch origin/main and pin this host to its agents.json", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := findRouterRoot()
		if err != nil {
			return err
		}
		m, err := router.SyncRegistrySnapshot(root, nil)
		if err != nil {
			return err
		}
		fmt.Printf("registry pinned to origin/main %s (sha256 %s…, fetched %s)\n", regShort(m.Commit), regShort(m.SHA256), m.FetchedAt)
		return nil
	},
}

var routerRegistryStatusCmd = &cobra.Command{
	Use: "status", Short: "Show which registry this host reads", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		root, err := findRouterRoot()
		if err != nil {
			return err
		}
		path, pinned, meta := router.RegistryPinStatus(root)
		if pinned && meta != nil {
			fmt.Printf("pinned to origin/main %s, fetched %s\n  reading %s\n", regShort(meta.Commit), meta.FetchedAt, path)
			return nil
		}
		fmt.Printf("reading the working tree: %s (not pinned; `sirsi router registry sync` to pin to origin/main)\n", path)
		return nil
	},
}

var routerRegistryUnpinCmd = &cobra.Command{
	Use: "unpin", Short: "Read the working-tree registry again", Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := router.UnpinRegistry(); err != nil {
			return err
		}
		fmt.Println("registry unpinned: reading the working tree")
		return nil
	},
}

func regShort(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func findRouterRoot() (string, error) {
	repo, err := router.FindRepoRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(repo, ".agents", "idea-router"), nil
}

func init() {
	routerRegistryCmd.AddCommand(routerRegistrySyncCmd, routerRegistryStatusCmd, routerRegistryUnpinCmd)
	routerCmd.AddCommand(routerRegistryCmd)
}
