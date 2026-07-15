package main

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/guard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/rtk"
	"github.com/SirsiMaster/sirsi-pantheon/internal/stele"
)

var (
	rtkMaxLines int
	rtkNoANSI   bool
	rtkNoDedup  bool
)

var rtkCmd = &cobra.Command{
	Use:   "rtk",
	Short: "Output filter — strip ANSI, dedup, truncate (subsumes RTK)",
	Long: `RTK — Output Filter

Filters terminal and tool output to reduce AI context window consumption.
Strips ANSI escape codes, deduplicates repeated lines, collapses blank runs,
and truncates oversized output with tail preservation.

  sirsi rtk filter < output.log    Filter stdin
  sirsi rtk stats < output.log     Show reduction statistics`,
}

var rtkFilterCmd = &cobra.Command{
	Use:   "filter",
	Short: "Filter output from stdin",
	RunE:  runRTKFilter,
}

var rtkStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show reduction statistics without outputting filtered text",
	RunE:  runRTKStats,
}

func init() {
	rtkFilterCmd.Flags().IntVar(&rtkMaxLines, "max-lines", 0, "Truncate after N lines (0 = unlimited)")
	rtkFilterCmd.Flags().BoolVar(&rtkNoANSI, "no-strip-ansi", false, "Don't strip ANSI escapes")
	rtkFilterCmd.Flags().BoolVar(&rtkNoDedup, "no-dedup", false, "Don't deduplicate lines")

	rtkStatsCmd.Flags().IntVar(&rtkMaxLines, "max-lines", 0, "Truncate after N lines (0 = unlimited)")

	rtkCmd.AddCommand(rtkFilterCmd, rtkStatsCmd)
}

func runRTKFilter(_ *cobra.Command, _ []string) error {
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	cfg := rtk.DefaultConfig()
	if rtkMaxLines > 0 {
		cfg.MaxLines = rtkMaxLines
	}
	if rtkNoANSI {
		cfg.StripANSI = false
	}
	if rtkNoDedup {
		cfg.Dedup = false
	}

	f := rtk.New(cfg)
	result := f.Apply(string(input))
	fmt.Print(result.Output)
	return nil
}

// rtkSavingsWindow bounds how much of the Stele the savings card reads. The
// ledger is append-only and unbounded, and this runs on a popover open.
const rtkSavingsWindow = 8 << 20

// renderRTKSavings reports what the filter has actually saved, from the Stele
// entries every filtered output inscribes. This is the no-stdin surface: the
// menubar row, and any bare `sirsi rtk stats`.
func renderRTKSavings() {
	res := &output.CommandResult{
		Command:    "sirsi rtk stats",
		BriefTitle: "RTK — Output Filter",
		Status:     "ok",
	}

	entries, err := stele.TailByType(stele.TypeRTKFilter, rtkSavingsWindow)
	switch {
	case err != nil:
		res.Status = "warn"
		res.Summary = "Sirsi cannot read the activity ledger, so it cannot show what the filter has saved."
		res.Errors = append(res.Errors, err.Error())
	case len(entries) == 0:
		// Honest empty state: the filter is armed, it just has not run yet.
		res.Summary = "The filter is on, but it has not trimmed anything yet. Numbers appear here once Sirsi has run commands for an agent."
	default:
		var original, filtered, dupes int
		for _, e := range entries {
			o, _ := strconv.Atoi(e.Data["original_bytes"])
			f, _ := strconv.Atoi(e.Data["filtered_bytes"])
			d, _ := strconv.Atoi(e.Data["dupes"])
			original += o
			filtered += f
			dupes += d
		}
		saved := original - filtered
		pct := 0.0
		if original > 0 {
			pct = float64(saved) / float64(original) * 100
		}
		res.Summary = fmt.Sprintf(
			"Sirsi trimmed %s of noise out of %s of tool output — %.0f%% less for the AI to read.",
			guard.FormatBytes(int64(saved)), guard.FormatBytes(int64(original)), pct)
		res.AddEvidence("Outputs filtered", strconv.Itoa(len(entries)))
		res.AddEvidence("Repeated lines removed", strconv.Itoa(dupes))
		res.AddEvidence("Measured over", "recent activity")
	}
	res.Render()
}

func runRTKStats(_ *cobra.Command, _ []string) error {
	// RTK is a PIPE filter — a live measurement needs piped input, and the
	// popover (like any tty caller) shells this with no stdin. It used to print
	// usage syntax here, which told the reader to go somewhere else and left
	// the screen dead. But every filtered output is inscribed on the Stele, so
	// report what RTK has ALREADY saved instead of explaining what it could.
	if fi, statErr := os.Stdin.Stat(); statErr == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
		renderRTKSavings()
		return nil
	}
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	cfg := rtk.DefaultConfig()
	if rtkMaxLines > 0 {
		cfg.MaxLines = rtkMaxLines
	}

	f := rtk.New(cfg)
	result := f.Apply(string(input))

	output.Banner()
	fmt.Printf("RTK Output Filter Statistics\n\n")
	fmt.Printf("  Original:  %d bytes\n", result.OriginalBytes)
	fmt.Printf("  Filtered:  %d bytes\n", result.FilteredBytes)
	fmt.Printf("  Reduction: %.1f%%\n", (1-result.Ratio)*100)
	fmt.Printf("  Lines removed: %d\n", result.LinesRemoved)
	fmt.Printf("  Duplicates collapsed: %d\n", result.DupsCollapsed)
	fmt.Printf("  Truncated: %v\n", result.Truncated)
	return nil
}
