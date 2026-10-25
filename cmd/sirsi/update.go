package main

import (
	"errors"
	"fmt"
	"net/http"
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
  sirsi update --cli    Open the exact signed Pantheon package in macOS Installer (CLI compatibility alias).
  sirsi update --app    Open the exact signed Pantheon package in macOS Installer.

The package handoff is not an installed-success claim. Ma'at retains an exact
update intent before opening Installer and reads the installed app, helper,
signatures, and required paths back before closing that attempt. An interrupted
or ambiguous handoff is reconciled by readback only; Installer is never replayed
automatically.`,
	RunE: runUpdate,
}

func runUpdate(_ *cobra.Command, _ []string) error {
	if updateInstallCLI || updateInstallApp {
		if handled, err := reconcilePendingMaatAppUpdate(); handled || err != nil {
			return err
		}
	}
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

// installAppRelease is the existing whole-app update owner. The platform
// adapter retains Ma'at preflight and an exact install intent before handing
// the verified commercial package to macOS Installer. A returned handoff is
// never treated as installed success; a later invocation must read the exact
// installed identity back before closing the retained attempt.
func installAppRelease(rel *updater.Release) error {
	if rel == nil {
		return fmt.Errorf("missing Pantheon release; installed product preserved")
	}
	// Check this final fetched record, not only the earlier availability read:
	// the release list can change between requests. Both --app and --cli use
	// this route, so neither can reopen a known-broken or older installer.
	if err := updater.ValidateUpdateVersion(version, rel.TagName); err != nil {
		return err
	}
	return installAppReleaseWithMaat(rel)
}

func init() {
	updateCmd.Flags().BoolVar(&updateInstallCLI, "cli", false, "open the verified unified Pantheon package in macOS Installer (compatibility alias)")
	updateCmd.Flags().BoolVar(&updateInstallApp, "app", false, "open the verified Pantheon package in macOS Installer")
	rootCmd.AddCommand(updateCmd)
}
