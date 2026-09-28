package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

var (
	updateInstallCLI bool
	updateInstallApp bool
	newUpdateClient  = updater.NewClient
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install the latest signed release",
	Long: `Check GitHub Releases for a newer Sirsi Pantheon and install it.

  sirsi update          Check only — report the newest release + any advisories.
  sirsi update --cli    Download the unified Pantheon app update (CLI compatibility alias).
  sirsi update --app    Download the notarized .app DMG and open it to install.

The .app is delivered as the notarized DMG so its Developer-ID signature — and
therefore your Full Disk Access grant — stays stable across updates. Ad-hoc
local rebuilds cannot do that; that is why merged fixes must arrive via a
signed release, not a hand-rebuilt binary.`,
	RunE: runUpdate,
}

func runUpdate(_ *cobra.Command, _ []string) error {
	// An explicit `sirsi update` tolerates a slower call than the 3s background
	// version-check, whose tight timeout trips on the full releases list. Use a
	// generous client for both the check and the release fetch.
	c := newUpdateClient()
	c.HTTPClient = &http.Client{Timeout: 20 * time.Second}

	res := c.Check(version)
	if res.Error != nil {
		if errors.Is(res.Error, updater.ErrNoCompleteCommercialRelease) {
			fmt.Println("  𓁢 No complete commercial Pantheon update is published yet; your installed version remains active.")
			fmt.Println("     Recovery: recheck later. A usable update requires the matching signed Pantheon DMG and PKG pair.")
			return nil
		}
		return fmt.Errorf("update check failed: %w", res.Error)
	}
	if notice := updater.FormatAdvisories(res.Advisories); notice != "" {
		fmt.Print(notice)
	}

	// Check-only (default): report and exit.
	if !updateInstallCLI && !updateInstallApp {
		if res.UpdateAvailable {
			fmt.Print(updater.FormatUpdateNotice(res))
			fmt.Println("     Install:  sirsi update --app   (unified CLI + menubar app, notarized)")
		} else {
			fmt.Printf("  𓁢 sirsi %s is current (latest release: %s)\n", res.CurrentVersion, res.LatestVersion)
		}
		return nil
	}

	rel, err := c.NewestRelease()
	if err != nil {
		return fmt.Errorf("fetch release: %w", err)
	}
	if updateInstallCLI {
		fmt.Println("  ↳ Commercial Pantheon updates ship one app payload; --cli is using the unified app installer.")
	}
	if updateInstallCLI || updateInstallApp {
		return installAppRelease(rel)
	}
	return nil
}

// installAppRelease downloads the notarized .app DMG and opens it so the user
// drags it into Applications. Installing the notarized bundle (vs. an ad-hoc
// rebuild) preserves the Developer-ID identity, so Full Disk Access survives.
func installAppRelease(rel *updater.Release) error {
	asset := updater.AppDMGAsset(rel)
	if asset == nil {
		return fmt.Errorf("release %s does not expose a complete arm64 Pantheon app payload", rel.TagName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dest := filepath.Join(home, "Downloads", asset.Name)
	fmt.Printf("  ↓ downloading %s …\n", asset.Name)
	if _, err = updater.Download(asset.BrowserDownloadURL, dest); err != nil {
		return err
	}

	fmt.Printf("  ✓ notarized app downloaded: %s\n", dest)
	fmt.Println("  Opening it — drag “Sirsi” onto Applications to install.")
	fmt.Println("  Its Developer-ID signature keeps Full Disk Access stable across future updates.")
	if err := exec.Command("open", dest).Run(); err != nil {
		return fmt.Errorf("open downloaded Pantheon DMG %q: %w; open this file manually to complete the unified app update", dest, err)
	}
	return nil
}

func init() {
	updateCmd.Flags().BoolVar(&updateInstallCLI, "cli", false, "download the unified Pantheon app update (compatibility alias)")
	updateCmd.Flags().BoolVar(&updateInstallApp, "app", false, "download the notarized .app DMG and open it to install")
	rootCmd.AddCommand(updateCmd)
}
