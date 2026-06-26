package main

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/selfupdate"
	"github.com/SirsiMaster/sirsi-pantheon/internal/updater"
)

var (
	updateInstallCLI bool
	updateInstallApp bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check for and install the latest signed release",
	Long: `Check GitHub Releases for a newer Sirsi Pantheon and install it.

  sirsi update          Check only — report the newest release + any advisories.
  sirsi update --cli    Install the latest CLI over this binary (AMFI-safe replace).
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
	c := updater.NewClient()
	c.HTTPClient = &http.Client{Timeout: 20 * time.Second}

	res := c.Check(version)
	if res.Error != nil {
		return fmt.Errorf("update check failed: %w", res.Error)
	}
	if notice := updater.FormatAdvisories(res.Advisories); notice != "" {
		fmt.Print(notice)
	}

	// Check-only (default): report and exit.
	if !updateInstallCLI && !updateInstallApp {
		if res.UpdateAvailable {
			fmt.Print(updater.FormatUpdateNotice(res))
			fmt.Println("     Install:  sirsi update --cli   (this CLI)")
			fmt.Println("               sirsi update --app   (menubar app, notarized)")
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
		if err := installCLIRelease(rel); err != nil {
			return err
		}
	}
	if updateInstallApp {
		if err := installAppRelease(rel); err != nil {
			return err
		}
	}
	return nil
}

// installCLIRelease downloads the release CLI tarball and replaces this binary
// in place using the existing AMFI-safe SafeReplace (rm + copy + re-sign).
func installCLIRelease(rel *updater.Release) error {
	asset := updater.CLITarballAsset(rel)
	if asset == nil {
		return fmt.Errorf("no CLI tarball for %s/%s in release %s", runtime.GOOS, runtime.GOARCH, rel.TagName)
	}

	tmp, err := os.MkdirTemp("", "sirsi-update-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	tarball := filepath.Join(tmp, asset.Name)
	fmt.Printf("  ↓ downloading %s …\n", asset.Name)
	if _, err = updater.Download(asset.BrowserDownloadURL, tarball); err != nil {
		return err
	}

	bin, err := updater.ExtractSirsiBinary(tarball, tmp)
	if err != nil {
		return err
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, e := filepath.EvalSymlinks(self); e == nil {
		self = resolved
	}

	if err = selfupdate.SafeReplace(bin, self); err != nil {
		return fmt.Errorf("install CLI over %s: %w", self, err)
	}
	fmt.Printf("  ✓ sirsi updated to %s at %s\n", rel.TagName, self)
	return nil
}

// installAppRelease downloads the notarized .app DMG and opens it so the user
// drags it into Applications. Installing the notarized bundle (vs. an ad-hoc
// rebuild) preserves the Developer-ID identity, so Full Disk Access survives.
func installAppRelease(rel *updater.Release) error {
	asset := updater.AppDMGAsset(rel)
	if asset == nil {
		return fmt.Errorf("no .app DMG for %s in release %s", runtime.GOARCH, rel.TagName)
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
	_ = exec.Command("open", dest).Run()
	return nil
}

func init() {
	updateCmd.Flags().BoolVar(&updateInstallCLI, "cli", false, "install the latest CLI over this binary (AMFI-safe)")
	updateCmd.Flags().BoolVar(&updateInstallApp, "app", false, "download the notarized .app DMG and open it to install")
	rootCmd.AddCommand(updateCmd)
}
