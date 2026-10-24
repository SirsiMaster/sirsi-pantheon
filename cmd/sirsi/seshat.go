package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/suggest"
)

var seshatDocs bool

var seshatCmd = &cobra.Command{
	Use:   "seshat",
	Short: "Legacy Ma'at knowledge-ingestion adapter (compatibility)",
	Long: `This hidden compatibility command preserves existing Seshat automation.

Ma'at is Pantheon's operator-facing knowledge and decision authority. Use
sirsi maat knowledge to inspect the retained, filtered library and
sirsi maat knowledge refresh to update it from configured local sources.

The legacy adapter retains compatible read/export verbs, but ingestion delegates
to Ma'at's canonical archive. Only the configured local Claude reader is
enabled here; unsupported adapters and profile flags refuse instead of reading
online sources or writing the retired raw snapshot store. Existing snapshots
remain migration inputs and are not modified.

  sirsi seshat ingest                 Refresh the configured local Claude source
  sirsi seshat ingest --source claude Refresh a specific enabled local source
  sirsi seshat ingest --profile       Ingest Chrome from a specific profile
  sirsi seshat ingest --all-profiles  Ingest Chrome from all profiles
  sirsi seshat export                 Export knowledge to a target
  sirsi seshat notebooklm             Export to NotebookLM + open browser
  sirsi seshat list                   List Knowledge Items
  sirsi seshat adapters               List available source/target adapters
  sirsi seshat profiles chrome        List available Chrome profiles
  sirsi seshat open chrome            Open Chrome with a specific profile
  sirsi seshat auth google            Authenticate with Google Workspace
  sirsi seshat mcp                    Start the MCP context server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if seshatDocs {
			output.Info("Opening Ma'at knowledge adapter docs...")
			return help.OpenDocs("seshat")
		}
		return cmd.Help()
	},
}

var seshatIngestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "Refresh Ma'at's canonical archive through the compatibility alias",
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		sourceName, _ := cmd.Flags().GetString("source")
		sinceStr, _ := cmd.Flags().GetString("since")
		exportTarget, _ := cmd.Flags().GetString("export")
		profileName, _ := cmd.Flags().GetString("profile")
		allProfiles, _ := cmd.Flags().GetBool("all-profiles")
		if exportTarget != "" {
			return errors.New("compatibility ingestion does not export; use the separate explicit Seshat export command")
		}
		if profileName != "" || allProfiles {
			return errors.New("Seshat profile ingestion is not enabled for Ma'at's canonical local reader")
		}
		since, err := parseMaatKnowledgeSince(sinceStr)
		if err != nil {
			return err
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return errors.New("Ma'at knowledge home unavailable")
		}
		if sourceName == "" {
			sourceName = "claude"
		}
		result, refreshErr := knowledge.RefreshLocal(cmd.Context(), home, sourceName, since, "")
		maatKnowledgeRefreshReceipt(time.Since(start), result).Render()
		if refreshErr != nil {
			return errors.New("Ma'at canonical knowledge refresh failed")
		}
		if result.State != knowledge.ImportComplete && result.State != knowledge.ImportNoNewItems {
			return errors.New("Ma'at canonical knowledge refresh was incomplete")
		}
		return nil
	},
}

var seshatExportCmd = &cobra.Command{
	Use:   "export <target>",
	Short: "𓁆 Export knowledge to a target system",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		output.Banner()
		output.Header("Knowledge Export")

		targetName := args[0]
		reg := seshat.DefaultRegistry()

		target, ok := reg.Targets[targetName]
		if !ok {
			return fmt.Errorf("unknown target '%s' — available: notebooklm, apple-notes, thoth", targetName)
		}

		// Load most recent ingestion
		items, err := loadLatestKnowledgeItems()
		if err != nil {
			return fmt.Errorf("no ingested data — run 'sirsi seshat ingest' first: %w", err)
		}

		fmt.Printf("  📤 Exporting %d items to %s...\n", len(items), target.Name())
		if err := target.Export(items); err != nil {
			return err
		}

		output.Success("Export complete")
		output.Footer(time.Since(start))
		return nil
	},
}

var seshatListCmd = &cobra.Command{
	Use:   "list",
	Short: "𓁆 List ingested Knowledge Items",
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()

		items, err := loadLatestKnowledgeItems()
		if err != nil {
			return errors.New("Ma'at canonical knowledge archive is unavailable; legacy reads are migration-only")
		}

		// CommandResult contract (owner surface law 2026-07-09): structured
		// list + a real lever (sync/ingest), not a raw Printf dump.
		res := &output.CommandResult{Command: "sirsi seshat list", BriefTitle: "Knowledge Library"}
		if len(items) == 0 {
			res.Summary = "No knowledge items ingested yet — the library is empty."
			res.Status = "ok"
			res.NextActions = append(res.NextActions,
				output.NextAction{Label: "Refresh local knowledge", Command: "sirsi maat knowledge refresh", Description: "Refresh the configured local source into Ma'at's canonical archive."},
			)
			res.Duration = time.Since(start)
			res.Render()
			return nil
		}
		res.Summary = fmt.Sprintf("%d knowledge item(s) in the library.", len(items))
		res.Status = "ok"
		for i, ki := range items {
			source := "unknown"
			for _, ref := range ki.References {
				if ref.Type == "source" {
					source = ref.Value
					break
				}
			}
			if i >= 50 {
				res.AddEvidence("…", fmt.Sprintf("+%d more — sirsi seshat list shows all", len(items)-50))
				break
			}
			res.AddEvidence(fmt.Sprintf("[%s] %s", source, ki.Title), output.Truncate(ki.Summary, 160))
		}
		// Source versions are immutable, so this alias does not offer destructive
		// prune or legacy dual-store synchronization.
		res.NextActions = append(res.NextActions,
			output.NextAction{Label: "Refresh local knowledge", Command: "sirsi maat knowledge refresh", Description: "Refresh the configured local source into Ma'at's canonical archive."},
		)
		res.Duration = time.Since(start)
		res.Render()
		return nil
	},
}

var seshatPruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Refuse destructive edits to Ma'at's immutable knowledge archive",
	Long: `𓁆 Prune the knowledge library.

	The canonical Ma'at archive retains immutable source history. Use filters to
	change a view; source history is never pruned through this compatibility alias.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("Ma'at knowledge versions are immutable; use search filters instead of pruning source history")
	},
}

var seshatAdaptersCmd = &cobra.Command{
	Use:   "adapters",
	Short: "𓁆 List available source and target adapters",
	Run: func(cmd *cobra.Command, args []string) {
		start := time.Now()
		output.Banner()
		output.Header("Adapter Registry")

		reg := seshat.DefaultRegistry()

		fmt.Println("  Source Adapters (ingest from):")
		for _, a := range reg.Sources {
			fmt.Printf("    %-20s %s\n", a.Name(), a.Description())
		}

		fmt.Println()
		fmt.Println("  Target Adapters (export to):")
		for _, a := range reg.Targets {
			fmt.Printf("    %-20s %s\n", a.Name(), a.Description())
		}

		output.Footer(time.Since(start))
		output.NextSteps(output.SuggestSteps(suggest.Context{Deity: "seshat", Subcommand: "adapters"}))
	},
}

var seshatSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "𓁆 Legacy: Bidirectional knowledge sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		return errors.New("legacy Seshat sync is disabled; Ma'at knowledge remains in its canonical archive")
	},
}

var seshatMcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "𓁆 Start the Model Context Protocol (MCP) context server",
	Run: func(cmd *cobra.Command, args []string) {
		// Keep this legacy alias protocol-clean and per-client as well. Writing
		// a banner to stdout corrupts JSON-RPC framing; a singleton lock would
		// deny other IDE sessions their own stdio transport.
		if err := runMCPServer(mcp.NewServer()); err != nil {
			output.Error("Server error: %v", err)
			os.Exit(1)
		}
	},
}

var seshatAuthCmd = &cobra.Command{
	Use:   "auth <provider>",
	Short: "𓁆 Authenticate with an external provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := args[0]
		switch provider {
		case "google":
			return seshatAuthGoogle()
		default:
			return fmt.Errorf("unknown provider '%s' — supported: google", provider)
		}
	},
}

func seshatAuthGoogle() error {
	output.Header("Google Workspace Authentication")

	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "seshat")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	credFile := filepath.Join(configDir, "google_credentials.json")
	tokenFile := filepath.Join(configDir, "google_token.json")

	if _, err := os.Stat(tokenFile); err == nil {
		output.Success("Google token already exists at %s", tokenFile)
		fmt.Println("  To re-authenticate, delete the token file and run again.")
		return nil
	}

	fmt.Println()
	fmt.Println("  To authenticate with Google Workspace:")
	fmt.Println()
	fmt.Println("  1. Go to https://console.cloud.google.com/apis/credentials")
	fmt.Println("  2. Create an OAuth 2.0 Client ID (Desktop application)")
	fmt.Println("  3. Enable these APIs: Google Drive API, Google Docs API, Google Sheets API")
	fmt.Println("  4. Download the credentials JSON")
	fmt.Printf("  5. Save it to: %s\n", credFile)
	fmt.Println()
	fmt.Printf("  Then run: sirsi seshat auth google\n")
	fmt.Println()

	if _, err := os.Stat(credFile); os.IsNotExist(err) {
		fmt.Printf("  ⚠️  No credentials found at %s\n", credFile)
		fmt.Println("     Follow the steps above to set up Google API access.")
		return nil
	}

	fmt.Println("  Credentials found. To complete auth, visit:")
	fmt.Println("  https://accounts.google.com/o/oauth2/auth?scope=https://www.googleapis.com/auth/drive.readonly&response_type=code&redirect_uri=urn:ietf:wg:oauth:2.0:oob&client_id=YOUR_CLIENT_ID")
	fmt.Println()
	fmt.Println("  Then paste the authorization code here (or save token manually to " + tokenFile + ")")

	return nil
}

// seshatProfilesCmd is the parent command for profile management.
var seshatProfilesCmd = &cobra.Command{
	Use:   "profiles",
	Short: "𓁆 Manage browser profiles for knowledge ingestion",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// seshatProfilesChromeCmd lists available Chrome profiles.
var seshatProfilesChromeCmd = &cobra.Command{
	Use:   "chrome",
	Short: "𓁆 List available Google Chrome profiles",
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		output.Banner()
		output.Header("Chrome Profiles")

		profiles, err := seshat.ListChromeProfiles()
		if err != nil {
			return fmt.Errorf("list Chrome profiles: %w", err)
		}

		if len(profiles) == 0 {
			output.Warn("No Chrome profiles found at %s", seshat.ChromeBaseDir())
			return nil
		}

		// Sort profiles by directory name for stable output
		sort.Slice(profiles, func(i, j int) bool {
			return profiles[i].DirName < profiles[j].DirName
		})

		headers := []string{"Directory", "Display Name", "Email", "Avatar"}
		var rows [][]string
		for _, p := range profiles {
			avatar := p.AvatarIcon
			if len(avatar) > 30 {
				avatar = avatar[:30] + "..."
			}
			rows = append(rows, []string{p.DirName, p.DisplayName, p.Email, avatar})
		}

		output.Table(headers, rows)
		fmt.Printf("\n  Found %d Chrome profiles in %s\n", len(profiles), output.ShortenPath(seshat.ChromeBaseDir()))
		output.Footer(time.Since(start))
		return nil
	},
}

// seshatChromeOpenCmd opens Chrome with a specific profile.
var seshatChromeOpenCmd = &cobra.Command{
	Use:   "chrome",
	Short: "𓁆 Open Chrome with a specific profile",
	Long: `Open Google Chrome with a specific profile.

The --profile flag accepts either a display name (e.g., "SirsiMaster") or
a directory name (e.g., "Profile 1"). Display names are resolved to directory
names using Chrome's Local State file.

Examples:
  sirsi seshat open chrome --profile SirsiMaster
  sirsi seshat open chrome --profile "Profile 1"
  sirsi seshat open chrome --profile Default --url https://notebooklm.google.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()
		output.Header("Chrome Launcher")

		profileName, _ := cmd.Flags().GetString("profile")
		url, _ := cmd.Flags().GetString("url")

		if profileName == "" {
			profileName = "Default"
		}

		dirName, err := seshat.OpenChromeWithProfile(profileName, url)
		if err != nil {
			return fmt.Errorf("open Chrome: %w", err)
		}

		output.Success("Launched Chrome with profile '%s' (directory: %s)", profileName, dirName)
		if url != "" {
			fmt.Printf("  🌐 URL: %s\n", url)
		}
		return nil
	},
}

// seshatOpenCmd is the parent command for opening applications.
var seshatOpenCmd = &cobra.Command{
	Use:   "open",
	Short: "𓁆 Open external applications with Seshat context",
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

// seshatExportNotebookLMCmd exports knowledge items and opens NotebookLM.
var seshatExportNotebookLMCmd = &cobra.Command{
	Use:   "notebooklm",
	Short: "𓁆 Export knowledge items to NotebookLM and open browser",
	Long: `Export Seshat knowledge items as Markdown files and open NotebookLM in Chrome.

This command:
  1. Runs the NotebookLM adapter export (Markdown files)
  2. Lists all exported files
  3. Opens Chrome with the specified profile to https://notebooklm.google.com
  4. Instructs you to drag-and-drop the exported files into NotebookLM

Examples:
  sirsi seshat export notebooklm --profile SirsiMaster
  sirsi seshat export notebooklm --profile Default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		output.Banner()
		output.Header("NotebookLM Export")

		profileName, _ := cmd.Flags().GetString("profile")
		if profileName == "" {
			profileName = "Default"
		}

		// Step 1: Load latest knowledge items
		items, err := loadLatestKnowledgeItems()
		if err != nil {
			return fmt.Errorf("no ingested data — run 'sirsi seshat ingest' first: %w", err)
		}

		// Step 2: Export via NotebookLM adapter
		reg := seshat.DefaultRegistry()
		target, ok := reg.Targets["notebooklm"]
		if !ok {
			return fmt.Errorf("NotebookLM adapter not found in registry")
		}

		fmt.Printf("  📤 Exporting %d items to NotebookLM format...\n", len(items))
		if exportErr := target.Export(items); exportErr != nil {
			return fmt.Errorf("export to NotebookLM: %w", exportErr)
		}

		// Step 3: List exported files
		home, _ := os.UserHomeDir()
		exportDir := filepath.Join(home, ".config", "seshat", "notebooklm_export")
		entries, err := os.ReadDir(exportDir)
		if err != nil {
			return fmt.Errorf("read export directory: %w", err)
		}

		fmt.Printf("\n  📁 Exported files in %s:\n", output.ShortenPath(exportDir))
		for _, entry := range entries {
			if !entry.IsDir() {
				info, _ := entry.Info()
				size := "0B"
				if info != nil {
					size = formatSize(info.Size())
				}
				fmt.Printf("     %s  %s\n", entry.Name(), output.DimStyle.Render(size))
			}
		}

		// Step 4: Open Chrome to NotebookLM
		fmt.Println()
		dirName, err := seshat.OpenChromeWithProfile(profileName, "https://notebooklm.google.com")
		if err != nil {
			output.Warn("Could not launch Chrome: %v", err)
			fmt.Println("  Open manually: https://notebooklm.google.com")
		} else {
			output.Success("Launched Chrome [%s] to NotebookLM", dirName)
		}

		// Step 5: Instructions
		fmt.Println()
		fmt.Println("  " + output.TitleStyle.Render("Next Steps:"))
		fmt.Println("  1. Create a new notebook in NotebookLM (or open an existing one)")
		fmt.Println("  2. Click 'Add source' in the notebook")
		fmt.Printf("  3. Drag-and-drop the files from: %s\n", output.ShortenPath(exportDir))
		fmt.Println("  4. NotebookLM will index the sources for grounded Q&A")

		output.Footer(time.Since(start))
		return nil
	},
}

// formatSize formats a byte count as a human-readable string.
func formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
	)
	switch {
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(MB))
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(KB))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func init() {
	seshatCmd.Flags().BoolVar(&seshatDocs, "docs", false, "Open Seshat web documentation in browser")

	seshatIngestCmd.Flags().String("source", "", "Specific source adapter (e.g., chrome-history, gemini, claude, apple-notes, google-workspace)")
	seshatIngestCmd.Flags().String("since", "", "Ingest items since (duration: '168h' or date: '2026-01-01')")
	seshatIngestCmd.Flags().String("export", "", "Auto-export to target after ingestion (e.g., thoth, notebooklm, apple-notes)")
	seshatIngestCmd.Flags().String("profile", "", "Chrome profile name or display name (default: 'Default')")
	seshatIngestCmd.Flags().Bool("all-profiles", false, "Ingest Chrome history from all available profiles")

	seshatSyncCmd.Flags().String("ki", "", "Knowledge Item name to sync")
	seshatSyncCmd.Flags().String("target", "", "Target GEMINI.md file path")

	seshatPruneCmd.Flags().String("source", "", "Prune only items from this source (e.g. chrome-history)")
	seshatPruneCmd.Flags().Bool("all", false, "Remove every ingested item")

	seshatChromeOpenCmd.Flags().String("profile", "", "Chrome profile name or display name (default: 'Default')")
	seshatChromeOpenCmd.Flags().String("url", "", "URL to open in Chrome")

	seshatExportNotebookLMCmd.Flags().String("profile", "", "Chrome profile for opening NotebookLM (default: 'Default')")

	// Wire subcommand trees
	seshatProfilesCmd.AddCommand(seshatProfilesChromeCmd)
	seshatOpenCmd.AddCommand(seshatChromeOpenCmd)

	seshatCmd.AddCommand(seshatIngestCmd)
	seshatCmd.AddCommand(seshatExportCmd)
	seshatCmd.AddCommand(seshatExportNotebookLMCmd)
	seshatCmd.AddCommand(seshatListCmd)
	seshatCmd.AddCommand(seshatPruneCmd)
	seshatCmd.AddCommand(seshatAdaptersCmd)
	seshatCmd.AddCommand(seshatProfilesCmd)
	seshatCmd.AddCommand(seshatOpenCmd)
	seshatCmd.AddCommand(seshatSyncCmd)
	seshatCmd.AddCommand(seshatMcpCmd)
	seshatCmd.AddCommand(seshatAuthCmd)
}

// loadLatestKnowledgeItems is a bounded compatibility projection from the
// canonical archive. Historical Seshat snapshots are migration-only and are
// never a fallback authority for current reads.
func loadLatestKnowledgeItems() ([]seshat.KnowledgeItem, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("Ma'at knowledge home unavailable")
	}
	archive, err := knowledge.Open(home)
	if err != nil {
		return nil, err
	}
	defer archive.Close()
	page, err := archive.Search(context.Background(), knowledge.SearchQuery{Limit: 100})
	if err != nil {
		return nil, err
	}
	if page.NextCursor != "" {
		return nil, errors.New("canonical knowledge exceeds the bounded compatibility export; use paginated Ma'at history")
	}
	items := make([]seshat.KnowledgeItem, 0, len(page.Items))
	for _, projected := range page.Items {
		if projected.WithheldReason != "" {
			continue
		}
		detail, err := archive.Detail(context.Background(), projected.ID, projected.Revision)
		if err != nil {
			return nil, err
		}
		items = append(items, seshat.KnowledgeItem{
			ID: detail.ID, Title: detail.Title, Summary: detail.Summary, Content: detail.Content,
			References: detail.References,
		})
	}
	return items, nil
}
