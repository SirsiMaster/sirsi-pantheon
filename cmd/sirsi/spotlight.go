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
var spotlightMarkersRemove bool

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
	spotlightExcludeCmd.Flags().BoolVar(&spotlightMarkersRemove, "remove", false, "with --markers: plan removal of the markers instead (preview unless --confirm)")
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
	plan := guard.PlanIndexMarkers(home, devDir)

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

	if spotlightMarkersRemove {
		return runSpotlightMarkerRemoval(plan, already, missing)
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
		output.Dim("     Re-run with --markers --confirm to write them. Reverse with --markers --remove.")
		printRootOnlyLevers()
		return nil
	}

	plan, applyErr := guard.ApplyIndexMarkers(plan)
	wrote := 0
	for _, st := range plan {
		if st.WroteNow {
			wrote++
			output.Success("  marked %s", st.Path)
		}
	}
	output.Info("\n  %d newly marked, %d already marked, %d absent.", wrote, already, missing)
	output.Dim("     Source trees are deliberately NOT marked — code search is the one Spotlight result worth keeping.")
	printSkipped(plan)
	printRootOnlyLevers()
	return applyErr
}

// runSpotlightMarkerRemoval is the reverse gesture, and it previews by default
// for the same reason apply does — except the stakes are inverted. Marking is
// additive; removal hands whole trees back to the indexer and will produce real
// reindex load, so the operator sees the complete list of affected paths, one
// line each, before anything is unlinked.
func runSpotlightMarkerRemoval(plan []guard.MarkerState, already, missing int) error {
	if !spotlightMarkersConfirm {
		output.Header("Spotlight index markers — removal preview (nothing unlinked)")
		for _, st := range plan {
			if !st.Exists || !st.Marked {
				continue
			}
			output.Info("  would unmark    %s", st.Path)
		}
		output.Info("\n  %d marker(s) to remove, %d absent.", already, missing)
		output.Dim("     Re-run with --markers --remove --confirm to remove them.")
		output.Dim("     Each removal returns that tree to Spotlight and will cost a reindex of it.")
		return nil
	}

	plan, removeErr := guard.RemoveIndexMarkers(plan)
	removed := 0
	for _, st := range plan {
		if st.Removed {
			removed++
			output.Success("  unmarked %s", st.Path)
		}
	}
	output.Info("\n  %d marker(s) removed of %d found.", removed, already)
	output.Dim("     Those trees are indexable again; expect reindex load as mds catches up.")
	printSkipped(plan)
	return removeErr
}

// printSkipped surfaces paths the write path deliberately refused. A refusal
// that is not printed is indistinguishable from a success, which is how a
// partial apply gets reported as a complete one.
func printSkipped(plan []guard.MarkerState) {
	for _, st := range plan {
		if st.Skipped != "" {
			output.Warn("  skipped %s — %s", st.Path, st.Skipped)
		}
	}
}

// printRootOnlyLevers states the part this process cannot do. Measured, not
// assumed, and the claim is scoped to what was measured: renice and
// taskpolicy -b were both tried here and both return EPERM, because mds is
// root-owned and mds_stores runs as _mds_stores. Those are the two mechanisms
// tested; no claim is made about mechanisms that were not.
func printRootOnlyLevers() {
	output.Dim("\n  The two mechanisms measured here both need root (renice and taskpolicy -b return EPERM; mds is root-owned):")
	for _, l := range guard.RootOnlyLevers() {
		output.Dim("     %s", l)
	}
}
