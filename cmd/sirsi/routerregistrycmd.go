package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/router"
	"github.com/SirsiMaster/sirsi-pantheon/internal/setup"
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
		if routerRegistryInstallSync {
			return installRegistrySync()
		}
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

var routerRegistryInstallSync bool

// installRegistrySync schedules `registry sync` hourly through launchd (no resident
// process) so a host never drifts from origin/main by omission. It pins the host the
// first time it runs.
func installRegistrySync() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	bin := setup.BinaryPath()
	if bin == "" || bin == "sirsi" {
		return fmt.Errorf("sirsi binary path could not be resolved")
	}
	repo, err := router.FindRepoRoot()
	if err != nil {
		return err
	}
	label := "ai.sirsi.registry-sync"
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>AssociatedBundleIdentifiers</key>
	<array>
		<string>ai.sirsi.pantheon</string>
	</array>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>router</string>
		<string>registry</string>
		<string>sync</string>
	</array>
	<key>WorkingDirectory</key>
	<string>%s</string>
	<key>StartInterval</key>
	<integer>3600</integer>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Background</string>
	<key>StandardOutPath</key>
	<string>%s/.sirsi/logs/registry-sync.log</string>
	<key>StandardErrorPath</key>
	<string>%s/.sirsi/logs/registry-sync.log</string>
</dict>
</plist>
`, label, bin, repo, home, home)
	path := filepath.Join(home, "Library", "LaunchAgents", label+".plist")
	if err := os.WriteFile(path, []byte(plist), 0o644); err != nil {
		return err
	}
	uid := fmt.Sprintf("gui/%d", os.Getuid())
	_ = exec.Command("launchctl", "bootout", uid+"/"+label).Run()
	if out, err := exec.Command("launchctl", "bootstrap", uid, path).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %w (%s)", err, out)
	}
	fmt.Printf("installed %s: re-pins this host to origin/main every hour (no resident process)\n", label)
	return nil
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
	routerRegistrySyncCmd.Flags().BoolVar(&routerRegistryInstallSync, "install", false, "Schedule an hourly sync through launchd instead of syncing now")
	routerRegistryCmd.AddCommand(routerRegistrySyncCmd, routerRegistryStatusCmd, routerRegistryUnpinCmd)
	routerCmd.AddCommand(routerRegistryCmd)
}
