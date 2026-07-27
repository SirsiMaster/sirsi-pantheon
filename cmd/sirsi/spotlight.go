package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/spf13/cobra"
)

var spotlightExcludeJSON bool
var spotlightMarkers bool
var spotlightMarkersConfirm bool

// spotlightPrivacyURL deep-links to System Settings ▸ Spotlight ▸ Privacy. The
// pane id churned across macOS versions; the extension form is Ventura+.
const spotlightPrivacyURL = "x-apple.systempreferences:com.apple.Spotlight-Settings.extension?Privacy"

var spotlightExcludeCmd = &cobra.Command{
	Use:   "spotlight-exclude [path]",
	Short: "Stop the Spotlight mds_stores storm by excluding a heavy-write dir (detect + guide)",
	Long: `Spotlight-Exclude — stop the write-amplification storm.

Agent file-write bursts in a heavy dev tree (default ~/Development) trigger
Spotlight reindexing that pins mds_stores/mdworker and feeds the RAM-pressure →
Jetsam loop. Excluding that folder from Spotlight Privacy stops it.

macOS deliberately makes Spotlight Privacy a user action (the on-disk Privacy
list is SIP-protected and version-sensitive). So this command does NOT mutate
system state — it detects the storm, then opens System Settings ▸ Spotlight ▸
Privacy and tells you the exact folder to add. Reverse it the same way (remove
the folder in that pane). Re-run ` + "`sirsi diagnose`" + ` afterward — the storm
signal subsiding is the proof it worked.

  sirsi spotlight-exclude              # guide for ~/Development
  sirsi spotlight-exclude ~/code       # guide for a specific dir
  sirsi spotlight-exclude --json       # read-only storm state (never opens UI)`,
	Args: cobra.MaximumNArgs(1),
	RunE: runSpotlightExclude,
}

func runSpotlightExclude(_ *cobra.Command, args []string) error {
	path := defaultDevDir()
	if len(args) == 1 && args[0] != "" {
		path = args[0]
	}
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}

	if spotlightMarkers {
		return runSpotlightMarkers(path)
	}

	// Read-only storm state from the shipped detector.
	report, err := guard.Doctor()
	if err != nil {
		return fmt.Errorf("read system health: %w", err)
	}
	storm := findSpotlightStorm(report.Findings)

	if spotlightExcludeJSON {
		out := map[string]any{
			"path":        path,
			"storming":    storm != nil && storm.Severity >= guard.SeverityWarn,
			"detail":      detailOf(storm),
			"privacy_url": spotlightPrivacyURL,
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
		return nil
	}

	// Idempotent on the actionable condition: if Spotlight isn't storming,
	// there's nothing to do (we never claim to have changed a system pref).
	if storm == nil || storm.Severity < guard.SeverityWarn {
		output.Success("Spotlight isn't storming right now — nothing to exclude.")
		output.Dim("     If the storm recurs (sirsi diagnose shows it), re-run this to get the exclusion guide.")
		return nil
	}

	output.Header("Spotlight write-amplification storm")
	output.Warn("  %s", storm.Message)
	if d := detailOf(storm); d != "" {
		output.Dim("     %s", d)
	}
	output.Info("\n  Fix: exclude %s from Spotlight indexing.", path)
	output.Dim("     Tradeoff: Spotlight search won't find files inside %s (most dev trees don't need it).", path)
	output.Dim("     This is reversible — remove the folder from the same Privacy list any time.")

	if runtime.GOOS != "darwin" {
		output.Dim("\n  (Spotlight is macOS-only — nothing to open on this platform.)")
		return nil
	}

	if !confirmFix("Open System Settings ▸ Spotlight ▸ Privacy now?") {
		output.Dim("     → not opened. To do it manually: System Settings ▸ Spotlight ▸ Privacy ▸ + , add %s", path)
		return nil
	}

	if err := exec.Command("open", spotlightPrivacyURL).Run(); err != nil {
		// Fallback to the top-level Spotlight pane if the Privacy anchor name
		// has changed on this macOS version.
		_ = exec.Command("open", "x-apple.systempreferences:com.apple.Spotlight-Settings.extension").Run()
	}
	output.Success("     Opened Spotlight settings.")
	output.Dim("     Now: drag %s into the Privacy list (or click + and choose it), then close.", path)
	output.Dim("     Verify: re-run `sirsi diagnose` — the Spotlight Storm signal should subside.")
	return nil
}

// defaultDevDir returns ~/Development, the canonical heavy-write tree.
func defaultDevDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "Development"
	}
	return filepath.Join(home, "Development")
}

func findSpotlightStorm(findings []guard.DiagnosticFinding) *guard.DiagnosticFinding {
	for i := range findings {
		if findings[i].Check == "Spotlight Storm" {
			return &findings[i]
		}
	}
	return nil
}

func detailOf(f *guard.DiagnosticFinding) string {
	if f == nil {
		return ""
	}
	return f.Detail
}

func init() {
	spotlightExcludeCmd.Flags().BoolVar(&spotlightExcludeJSON, "json", false, "read-only storm state as JSON (never opens the UI)")
	spotlightExcludeCmd.Flags().BoolVar(&spotlightMarkers, "markers", false, "plan .metadata_never_index markers over high-churn trees (preview)")
	spotlightExcludeCmd.Flags().BoolVar(&spotlightMarkersConfirm, "confirm", false, "with --markers: actually write the markers")
}

// runSpotlightMarkers is the durable, unprivileged half of the remediation.
// The Privacy pane is user-only and does not survive being described; a
// .metadata_never_index marker is a file this process can actually write, and
// re-running is a no-op rather than a duplicate.
func runSpotlightMarkers(devDir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home: %w", err)
	}
	plan := planIndexMarkers(home, devDir)

	var already, todo, missing int
	for _, st := range plan {
		switch {
		case !st.Exists:
			missing++
		case st.Marked:
			already++
		default:
			todo++
		}
	}

	if !spotlightMarkersConfirm {
		output.Header("Spotlight index markers — preview (nothing written)")
		for _, st := range plan {
			switch {
			case !st.Exists:
				continue
			case st.Marked:
				output.Dim("  already marked  %s", st.Path)
			default:
				output.Info("  would mark      %s", st.Path)
			}
		}
		output.Info("\n  %d to mark, %d already marked, %d absent.", todo, already, missing)
		output.Dim("     Re-run with --markers --confirm to write them. Reverse by deleting %s.", markerFile)
		printRootOnlyLevers()
		return nil
	}

	plan, applyErr := applyIndexMarkers(plan)
	wrote := 0
	for _, st := range plan {
		if st.WroteNow {
			wrote++
			output.Success("  marked %s", st.Path)
		}
	}
	output.Info("\n  %d newly marked, %d already marked, %d absent.", wrote, already, missing)
	output.Dim("     Source trees are deliberately NOT marked — code search is the one Spotlight result worth keeping.")
	printRootOnlyLevers()
	return applyErr
}

// printRootOnlyLevers states the part this process cannot do. Measured, not
// assumed: mds runs as root and mds_stores as _mds_stores, so renice and
// taskpolicy both return EPERM here.
func printRootOnlyLevers() {
	output.Dim("\n  Bounding the indexer itself needs root (mds is root-owned; renice/taskpolicy return EPERM):")
	for _, l := range rootOnlyLevers() {
		output.Dim("     %s", l)
	}
}
