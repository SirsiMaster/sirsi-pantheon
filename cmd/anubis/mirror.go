package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-anubis/internal/mirror"
	"github.com/SirsiMaster/sirsi-anubis/internal/output"
)

var (
	mirrorPhotos  bool
	mirrorMusic   bool
	mirrorMinSize string
	mirrorMaxSize string
	mirrorDryRun  bool
	mirrorProtect []string
)

var mirrorCmd = &cobra.Command{
	Use:   "mirror [directories...]",
	Short: "🪞 Find duplicate files with smart keep/delete recommendations",
	Long: `🪞 Mirror — The Sacred Reflection

Egyptian copper mirrors reveal truth — what is real and what is merely
a reflection. Mirror scans your files and finds duplicates, recommending
which copy to keep based on location, age, and context.

  anubis mirror                           Launch the visual GUI
  anubis mirror ~/Photos ~/Downloads      CLI scan across directories
  anubis mirror --photos ~/Pictures       Photo-specific scan (jpg, heic, png...)
  anubis mirror --music ~/Music           Music-specific scan (mp3, flac, m4a...)
  anubis mirror --min-size 1MB            Skip files smaller than 1MB
  anubis mirror --protect ~/Originals     Never suggest deleting from this dir

How it works:
  1. Groups files by size (instant pre-filter)
  2. Hashes only same-size files with SHA-256 (confirms exact matches)
  3. Recommends which duplicate to keep (protected > shallow > oldest > largest)

All scanning is read-only. Mirror never deletes files without --confirm.

Pro tier: Importance ranking with on-device neural analysis (ANE).`,
	Args: cobra.ArbitraryArgs,
	Run:  runMirror,
}

func init() {
	mirrorCmd.Flags().BoolVar(&mirrorPhotos, "photos", false, "Scan only photo files (jpg, heic, png, raw...)")
	mirrorCmd.Flags().BoolVar(&mirrorMusic, "music", false, "Scan only music files (mp3, flac, m4a...)")
	mirrorCmd.Flags().StringVar(&mirrorMinSize, "min-size", "", "Minimum file size (e.g., 1KB, 5MB, 1GB)")
	mirrorCmd.Flags().StringVar(&mirrorMaxSize, "max-size", "", "Maximum file size (e.g., 100MB, 1GB)")
	mirrorCmd.Flags().BoolVar(&mirrorDryRun, "dry-run", true, "Preview mode — show duplicates without action (default: true)")
	mirrorCmd.Flags().StringSliceVar(&mirrorProtect, "protect", nil, "Directories whose files should never be suggested for deletion")
}

func runMirror(cmd *cobra.Command, args []string) {
	// No args = launch GUI
	if len(args) == 0 {
		output.Header("🪞 Mirror — Launching Visual UI")
		fmt.Println()

		srv, srvErr := mirror.NewServer()
		if srvErr != nil {
			output.Error(fmt.Sprintf("Failed to start server: %v", srvErr))
			os.Exit(1)
		}

		output.Info(fmt.Sprintf("  🌐 %s", srv.URL()))
		output.Info("  Opening browser...")
		fmt.Println()
		output.Info("  Press Ctrl+C to stop")
		fmt.Println()

		if openErr := srv.OpenBrowser(); openErr != nil {
			output.Warn(fmt.Sprintf("  Could not open browser: %v", openErr))
			output.Info(fmt.Sprintf("  Open manually: %s", srv.URL()))
		}

		if serveErr := srv.Serve(); serveErr != nil {
			output.Error(fmt.Sprintf("Server error: %v", serveErr))
			os.Exit(1)
		}
		return
	}

	// With args = CLI mode
	output.Header("🪞 Mirror — Duplicate File Scanner")
	fmt.Println()

	opts := mirror.ScanOptions{
		Paths:       args,
		DryRun:      mirrorDryRun,
		ProtectDirs: mirrorProtect,
	}

	if mirrorPhotos {
		opts.MediaFilter = mirror.MediaPhoto
		output.Info("📷 Scanning photos only")
	} else if mirrorMusic {
		opts.MediaFilter = mirror.MediaMusic
		output.Info("🎵 Scanning music only")
	}

	if mirrorMinSize != "" {
		size, err := parseSize(mirrorMinSize)
		if err != nil {
			output.Error(fmt.Sprintf("Invalid --min-size: %v", err))
			os.Exit(1)
		}
		opts.MinSize = size
	}
	if mirrorMaxSize != "" {
		size, err := parseSize(mirrorMaxSize)
		if err != nil {
			output.Error(fmt.Sprintf("Invalid --max-size: %v", err))
			os.Exit(1)
		}
		opts.MaxSize = size
	}

	// Show scan targets
	for _, p := range args {
		abs, _ := filepath.Abs(p)
		output.Info(fmt.Sprintf("  📂 %s", abs))
	}
	fmt.Println()
	output.Info("Scanning...")
	fmt.Println()

	result, err := mirror.Scan(opts)
	if err != nil {
		output.Error(fmt.Sprintf("Scan failed: %v", err))
		os.Exit(1)
	}

	if jsonOutput {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(result)
		return
	}

	renderMirrorResult(result)
}

func renderMirrorResult(r *mirror.MirrorResult) {
	// Summary
	output.Info(fmt.Sprintf("  Files scanned:   %d", r.TotalScanned))
	output.Info(fmt.Sprintf("  Unique files:    %d", r.UniqueFiles))
	output.Info(fmt.Sprintf("  Duplicates:      %d", r.TotalDuplicates))
	output.Info(fmt.Sprintf("  Wasted space:    %s", mirror.FormatBytes(r.TotalWasteBytes)))
	output.Info(fmt.Sprintf("  Scan time:       %s", r.ScanDuration.Round(1e6)))
	fmt.Println()

	if len(r.Groups) == 0 {
		output.Info("✅ No duplicates found — your files are clean!")
		fmt.Println()
		return
	}

	output.Warn(fmt.Sprintf("🔍 Found %d duplicate groups — %s recoverable",
		len(r.Groups), mirror.FormatBytes(r.TotalWasteBytes)))
	fmt.Println()

	// Show top duplicate groups (limit to 20 for readability)
	limit := len(r.Groups)
	if limit > 20 {
		limit = 20
	}

	for i, g := range r.Groups[:limit] {
		fmt.Printf("  ── Group %d (%s match, %s wasted) ──\n",
			i+1, g.MatchType, mirror.FormatBytes(g.WasteBytes))

		for j, f := range g.Files {
			var marker string
			if j == g.Recommended {
				marker = "✓ "
			} else {
				marker = "✗ "
			}

			path := shortenPath(f.Path)
			mediaIcon := mediaIcon(f.MediaType)

			protectedTag := ""
			if f.IsProtected {
				protectedTag = " 🔒"
			}

			fmt.Printf("    %s%s %s  (%s, %s)%s\n",
				marker, mediaIcon, path,
				mirror.FormatBytes(f.Size),
				f.ModTime.Format("2006-01-02"),
				protectedTag)
		}
		fmt.Println()
	}

	if len(r.Groups) > limit {
		output.Info(fmt.Sprintf("  ... and %d more groups (use --json for full list)", len(r.Groups)-limit))
		fmt.Println()
	}

	// Legend
	output.Info("  ✓ = recommended to keep")
	output.Info("  ✗ = safe to remove")
	output.Info("  🔒 = in protected directory")
	fmt.Println()
	output.Info("💡 Pro tier: importance ranking with on-device neural analysis")
	output.Info("   → anubis install-brain && anubis mirror --rank")
	fmt.Println()
}

func mediaIcon(mt mirror.MediaType) string {
	switch mt {
	case mirror.MediaPhoto:
		return "📷"
	case mirror.MediaMusic:
		return "🎵"
	case mirror.MediaVideo:
		return "🎬"
	case mirror.MediaDocument:
		return "📄"
	default:
		return "📁"
	}
}

// parseSize converts a human-readable size to bytes.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))

	multipliers := map[string]int64{
		"B":  1,
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSuffix(s, suffix)
			numStr = strings.TrimSpace(numStr)
			var num float64
			if _, scanErr := fmt.Sscanf(numStr, "%f", &num); scanErr != nil {
				return 0, fmt.Errorf("invalid number: %s", numStr)
			}
			return int64(num * float64(mult)), nil
		}
	}

	var num int64
	if _, scanErr := fmt.Sscanf(s, "%d", &num); scanErr != nil {
		return 0, fmt.Errorf("invalid size: %s (use e.g. 1KB, 5MB, 1GB)", s)
	}
	return num, nil
}
