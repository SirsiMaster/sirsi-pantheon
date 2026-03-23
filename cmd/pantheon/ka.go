package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/horus"
	"github.com/SirsiMaster/sirsi-pantheon/internal/jackal"
	"github.com/SirsiMaster/sirsi-pantheon/internal/ka"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

// Ka flags
var (
	kaSudo    bool
	kaClean   bool
	kaDryRun  bool
	kaConfirm bool
	kaTarget  string
	kaDeep    bool
)

// kaCmd implements `pantheon ka` — the Ghost Hunter.
var kaCmd = &cobra.Command{
	Use:   "ka",
	Short: "𓂓 Hunt ghost apps — find the spirits of the dead",
	Long: `𓂓 Ka — The Ghost Hunter

In Egyptian belief, the Ka is the spiritual double that persists after
the body dies. When you uninstall an app, its Ka lingers — preferences,
caches, containers, launch agents, Spotlight registrations — consuming
resources and haunting your system.

Anubis Ka finds these spirits and releases them.

  pantheon ka                     Scan for all ghosts
  pantheon ka --deep               Include Spotlight/Launch Services ghosts
  pantheon ka --sudo              Include system-level ghosts (requires sudo)
  pantheon ka --clean --dry-run   Preview ghost cleanup
  pantheon ka --clean --confirm   Release the spirits (delete residuals)
  pantheon ka --target "Parallels"  Hunt a specific ghost by name`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runKa()
	},
}

func init() {
	kaCmd.Flags().BoolVar(&kaSudo, "sudo", false, "Include system-level residuals (requires sudo)")
	kaCmd.Flags().BoolVar(&kaDeep, "deep", false, "Include Spotlight/Launch Services ghost scan (slower)")
	kaCmd.Flags().BoolVar(&kaClean, "clean", false, "Clean ghost residuals (requires --dry-run or --confirm)")
	kaCmd.Flags().BoolVar(&kaDryRun, "dry-run", false, "Preview what would be cleaned")
	kaCmd.Flags().BoolVar(&kaConfirm, "confirm", false, "Actually clean ghost residuals")
	kaCmd.Flags().StringVar(&kaTarget, "target", "", "Hunt a specific ghost by name or bundle ID")
}

func runKa() error {
	if kaClean && !kaDryRun && !kaConfirm {
		output.Error("You must specify --dry-run or --confirm with --clean")
		output.Info("")
		output.Info("  pantheon ka --clean --dry-run    Preview ghost cleanup")
		output.Info("  pantheon ka --clean --confirm    Release the spirits")
		return fmt.Errorf("missing required flag: --dry-run or --confirm")
	}

	start := time.Now()

	if !quietMode {
		output.Banner()
		output.Header("𓂓 KA — THE GHOST HUNTER")
		output.Info("Searching for spirits of the dead...")
		fmt.Fprintln(os.Stderr)
	}

	scanner := ka.NewScanner()

	// Load Horus shared index for fast DirSizeAndCount lookups.
	if manifest, err := horus.Index(horus.IndexOptions{}); err == nil {
		scanner.Manifest = manifest
	}

	// Skip lsregister -dump unless --deep (saves ~5 seconds).
	scanner.SkipLaunchServices = !kaDeep

	ghosts, err := scanner.Scan(kaSudo)
	if err != nil {
		return fmt.Errorf("ka scan failed: %w", err)
	}

	// Filter by target if specified
	if kaTarget != "" {
		var filtered []ka.Ghost
		for _, g := range ghosts {
			if containsCI(g.AppName, kaTarget) || containsCI(g.BundleID, kaTarget) {
				filtered = append(filtered, g)
			}
		}
		ghosts = filtered
	}

	// Sort by total size (largest first)
	sort.Slice(ghosts, func(i, j int) bool {
		return ghosts[i].TotalSize > ghosts[j].TotalSize
	})

	// JSON output
	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(ghosts)
	}

	if len(ghosts) == 0 {
		output.Success("No ghosts detected. Your machine is spiritually clean. 𓂓")
		return nil
	}

	// Display ghosts
	output.Header(fmt.Sprintf("Found %d Ghosts (Dead App Remnants)", len(ghosts)))
	fmt.Fprintln(os.Stderr)

	var totalSize int64
	var totalResiduals int

	for i, ghost := range ghosts {
		totalSize += ghost.TotalSize
		totalResiduals += len(ghost.Residuals)

		// Ghost header
		lsIcon := ""
		if ghost.InLaunchServices {
			lsIcon = " 👻 (in Spotlight)"
		}

		sizeStr := ""
		if ghost.TotalSize > 0 {
			sizeStr = fmt.Sprintf(" — %s", jackal.FormatSize(ghost.TotalSize))
		}

		output.Info("%s %s%s%s",
			output.SizeStyle.Render(fmt.Sprintf("[%d]", i+1)),
			output.TitleStyle.Render(ghost.AppName),
			output.DimStyle.Render(sizeStr),
			lsIcon,
		)
		output.Dim("    Bundle: %s", ghost.BundleID)

		// Residual details
		for _, r := range ghost.Residuals {
			sizeLabel := jackal.FormatSize(r.SizeBytes)
			sudoLabel := ""
			if r.RequiresSudo {
				sudoLabel = " (sudo)"
			}
			output.Dim("    ├─ %-22s %8s  %s%s",
				string(r.Type),
				sizeLabel,
				shortenPath(r.Path),
				sudoLabel,
			)
		}
		fmt.Fprintln(os.Stderr)
	}

	// Summary
	output.Summary(
		jackal.FormatSize(totalSize),
		totalResiduals,
		len(ghosts),
	)
	output.Dim("  %d ghosts • %d residual hauntings • scanned in %s",
		len(ghosts),
		totalResiduals,
		time.Since(start).Round(time.Millisecond),
	)

	// Clean mode
	if kaClean {
		fmt.Fprintln(os.Stderr)
		if kaDryRun {
			output.Header("DRY RUN — Would release these spirits")
		} else {
			output.Header("RELEASING SPIRITS — Cleaning ghost residuals")
		}

		var totalFreed int64
		var totalCleaned int

		for _, ghost := range ghosts {
			freed, cleaned, err := scanner.Clean(ghost, kaDryRun, true)
			if err != nil {
				output.Error("Failed to release %s: %s", ghost.AppName, err)
				continue
			}
			totalFreed += freed
			totalCleaned += cleaned

			if kaDryRun {
				output.Info("Would release %s (%s, %d residuals)",
					ghost.AppName, jackal.FormatSize(freed), cleaned)
			} else {
				output.Success("Released %s (%s, %d residuals)",
					ghost.AppName, jackal.FormatSize(freed), cleaned)
			}
		}

		fmt.Fprintln(os.Stderr)
		if kaDryRun {
			output.Info("Would free %s across %d residuals",
				jackal.FormatSize(totalFreed), totalCleaned)
			output.Info("Run %s to release the spirits.",
				output.SizeStyle.Render("pantheon ka --clean --confirm"))
		} else {
			output.Success("Freed %s across %d residuals. The spirits are at rest. 𓂓",
				jackal.FormatSize(totalFreed), totalCleaned)
		}
	} else {
		fmt.Fprintln(os.Stderr)
		output.Info("Run %s to preview cleanup.",
			output.SizeStyle.Render("pantheon ka --clean --dry-run"))
	}

	return nil
}

func containsCI(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			len(substr) == 0 ||
			findCI(s, substr))
}

func findCI(s, substr string) bool {
	s = toLowerASCII(s)
	substr = toLowerASCII(substr)
	return len(s) >= len(substr) && contains(s, substr)
}

func toLowerASCII(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		b[i] = c
	}
	return string(b)
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
