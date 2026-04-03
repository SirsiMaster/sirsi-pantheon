package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/help"
	"github.com/SirsiMaster/sirsi-pantheon/internal/mcp"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
	"github.com/SirsiMaster/sirsi-pantheon/internal/platform"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

var seshatDocs bool

var seshatCmd = &cobra.Command{
	Use:   "seshat",
	Short: "𓁆 Seshat — Universal Knowledge Grafting Engine",
	Long: `𓁆 Seshat — Goddess of writing, wisdom, and measurement.

Seshat is the universal knowledge grafting layer. She ingests knowledge
from multiple sources (Chrome, Gemini, Claude, Apple Notes, Google Workspace),
reconciles it, and distributes to targets (Thoth, NotebookLM, Apple Notes).

  pantheon seshat ingest                 Ingest knowledge from all sources
  pantheon seshat ingest --source        Ingest from a specific source
  pantheon seshat ingest --profile       Ingest Chrome from a specific profile
  pantheon seshat ingest --all-profiles  Ingest Chrome from all profiles
  pantheon seshat export                 Export knowledge to a target
  pantheon seshat notebooklm             Export to NotebookLM + open browser
  pantheon seshat list                   List Knowledge Items
  pantheon seshat adapters               List available source/target adapters
  pantheon seshat profiles chrome        List available Chrome profiles
  pantheon seshat open chrome            Open Chrome with a specific profile
  pantheon seshat auth google            Authenticate with Google Workspace
  pantheon seshat mcp                    Start the MCP context server`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if seshatDocs {
			output.Info("Opening Seshat docs...")
			return help.OpenDocs("seshat")
		}
		return cmd.Help()
	},
}

var seshatIngestCmd = &cobra.Command{
	Use:   "ingest",
	Short: "𓁆 Ingest knowledge from external sources",
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		output.Banner()
		output.Header("SESHAT — Knowledge Ingestion")

		sourceName, _ := cmd.Flags().GetString("source")
		sinceStr, _ := cmd.Flags().GetString("since")
		exportTarget, _ := cmd.Flags().GetString("export")
		profileName, _ := cmd.Flags().GetString("profile")
		allProfiles, _ := cmd.Flags().GetBool("all-profiles")

		var since time.Time
		if sinceStr != "" {
			d, err := time.ParseDuration(sinceStr)
			if err != nil {
				// Try as date
				since, err = time.Parse("2006-01-02", sinceStr)
				if err != nil {
					return fmt.Errorf("invalid --since value: use duration (e.g., '7d', '24h') or date (2006-01-02)")
				}
			} else {
				since = time.Now().Add(-d)
			}
		} else {
			// Default: last 7 days
			since = time.Now().Add(-7 * 24 * time.Hour)
		}

		var items []seshat.KnowledgeItem

		// Handle --all-profiles: ingest Chrome history from every profile
		if allProfiles && (sourceName == "" || sourceName == "chrome-history") {
			profiles, err := seshat.ListChromeProfiles()
			if err != nil {
				return fmt.Errorf("list Chrome profiles: %w", err)
			}
			fmt.Printf("  📥 Ingesting Chrome history from %d profiles...\n", len(profiles))
			for _, prof := range profiles {
				profileDir, resolveErr := seshat.ResolveProfileDir(prof.DirName)
				if resolveErr != nil {
					fmt.Printf("  ⚠️  Skipping profile %s: %v\n", prof.DirName, resolveErr)
					continue
				}
				adapter := &seshat.ChromeHistoryAdapter{ProfileDir: profileDir}
				fmt.Printf("  📥 [%s] %s (%s)...\n", prof.DirName, prof.DisplayName, prof.Email)
				result, ingestErr := adapter.Ingest(since)
				if ingestErr != nil {
					fmt.Printf("  ⚠️  %s: %v\n", prof.DirName, ingestErr)
					continue
				}
				// Tag each item with which profile it came from
				for i := range result {
					result[i].References = append(result[i].References, seshat.KIReference{
						Type:  "chrome_profile",
						Value: prof.DirName,
					})
				}
				items = append(items, result...)
			}
			// If sourceName was empty, also ingest from non-Chrome sources
			if sourceName == "" {
				reg := seshat.DefaultRegistry()
				for name, adapter := range reg.Sources {
					if name == "chrome-history" {
						continue // already handled above
					}
					result, err := adapter.Ingest(since)
					if err != nil {
						fmt.Printf("  ⚠️  %s: %v\n", name, err)
						continue
					}
					items = append(items, result...)
				}
			}
		} else {
			reg := seshat.DefaultRegistry()

			// Apply --profile flag to the Chrome adapter
			if profileName != "" {
				profileDir, err := seshat.ResolveProfileDir(profileName)
				if err != nil {
					return fmt.Errorf("resolve profile: %w", err)
				}
				reg.Sources["chrome-history"] = &seshat.ChromeHistoryAdapter{ProfileDir: profileDir}
				fmt.Printf("  🔑 Using Chrome profile: %s (%s)\n", profileName, output.ShortenPath(profileDir))
			}

			if sourceName != "" {
				adapter, ok := reg.Sources[sourceName]
				if !ok {
					return fmt.Errorf("unknown source '%s' — run 'pantheon seshat adapters' for list", sourceName)
				}
				fmt.Printf("  📥 Ingesting from %s...\n", adapter.Name())
				result, err := adapter.Ingest(since)
				if err != nil {
					return fmt.Errorf("%s: %w", adapter.Name(), err)
				}
				items = result
			} else {
				fmt.Printf("  📥 Ingesting from all sources (since %s)...\n", since.Format("2006-01-02 15:04"))
				result, err := reg.IngestAll(since)
				if err != nil {
					return err
				}
				items = result
			}
		}

		fmt.Printf("  ✅ Ingested %d knowledge items\n", len(items))

		// Save to local store
		if len(items) > 0 {
			storePath, err := saveKnowledgeItems(items)
			if err != nil {
				fmt.Printf("  ⚠️  Save: %v\n", err)
			} else {
				fmt.Printf("  💾 Saved to %s\n", storePath)
			}
		}

		// Auto-export if --export specified
		if exportTarget != "" && len(items) > 0 {
			reg := seshat.DefaultRegistry()
			target, ok := reg.Targets[exportTarget]
			if !ok {
				return fmt.Errorf("unknown target '%s' — run 'pantheon seshat adapters' for list", exportTarget)
			}
			fmt.Printf("  📤 Exporting to %s...\n", target.Name())
			if err := target.Export(items); err != nil {
				return fmt.Errorf("export to %s: %w", target.Name(), err)
			}
		}

		output.Footer(time.Since(start))
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
		output.Header("SESHAT — Knowledge Export")

		targetName := args[0]
		reg := seshat.DefaultRegistry()

		target, ok := reg.Targets[targetName]
		if !ok {
			return fmt.Errorf("unknown target '%s' — available: notebooklm, apple-notes, thoth", targetName)
		}

		// Load most recent ingestion
		items, err := loadLatestKnowledgeItems()
		if err != nil {
			return fmt.Errorf("no ingested data — run 'pantheon seshat ingest' first: %w", err)
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
		output.Banner()
		output.Header("SESHAT — Knowledge Library")

		items, err := loadLatestKnowledgeItems()
		if err != nil {
			// Fall back to legacy Antigravity KIs
			paths := seshat.DefaultPaths()
			legacyItems, _ := seshat.ListKnowledgeItems(paths)
			for i, item := range legacyItems {
				ki, _ := seshat.ReadKnowledgeItem(paths, item)
				if ki != nil {
					fmt.Printf("  %d. %s\n", i+1, ki.Title)
					fmt.Printf("     %s\n", output.Truncate(ki.Summary, 80))
				}
			}
			output.Footer(time.Since(start))
			return nil
		}

		for i, ki := range items {
			source := "unknown"
			for _, ref := range ki.References {
				if ref.Type == "source" {
					source = ref.Value
					break
				}
			}
			fmt.Printf("  %d. [%s] %s\n", i+1, source, ki.Title)
			fmt.Printf("     %s\n", output.Truncate(ki.Summary, 80))
		}

		fmt.Printf("\n  Total: %d knowledge items\n", len(items))
		output.Footer(time.Since(start))
		return nil
	},
}

var seshatAdaptersCmd = &cobra.Command{
	Use:   "adapters",
	Short: "𓁆 List available source and target adapters",
	Run: func(cmd *cobra.Command, args []string) {
		output.Banner()
		output.Header("SESHAT — Adapter Registry")

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
	},
}

var seshatSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "𓁆 Legacy: Bidirectional knowledge sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		output.Banner()
		output.Header("SESHAT — Knowledge Sync")

		paths := seshat.DefaultPaths()
		kiName, _ := cmd.Flags().GetString("ki")
		target, _ := cmd.Flags().GetString("target")

		if kiName != "" && target != "" {
			if err := seshat.SyncKIToGeminiMD(paths, kiName, target); err != nil {
				return err
			}
			output.Success("Synced KI '%s' → %s", kiName, target)
		}

		output.Footer(time.Since(start))
		return nil
	},
}

var seshatMcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "𓁆 Start the Model Context Protocol (MCP) context server",
	Run: func(cmd *cobra.Command, args []string) {
		unlock, err := platform.TryLock("mcp-cli")
		if err != nil {
			output.Error("Pantheon MCP Server is already active.")
			return
		}
		defer unlock()

		output.Header("SESHAT — Scribe's Voice (MCP Server)")

		server := mcp.NewServer()
		if err := server.Run(); err != nil {
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
	output.Header("SESHAT — Google Workspace Authentication")

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
	fmt.Printf("  Then run: pantheon seshat auth google\n")
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
		output.Header("SESHAT — Chrome Profiles")

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
  pantheon seshat open chrome --profile SirsiMaster
  pantheon seshat open chrome --profile "Profile 1"
  pantheon seshat open chrome --profile Default --url https://notebooklm.google.com`,
	RunE: func(cmd *cobra.Command, args []string) error {
		output.Banner()
		output.Header("SESHAT — Chrome Launcher")

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
  pantheon seshat export notebooklm --profile SirsiMaster
  pantheon seshat export notebooklm --profile Default`,
	RunE: func(cmd *cobra.Command, args []string) error {
		start := time.Now()
		output.Banner()
		output.Header("SESHAT — NotebookLM Export + Launch")

		profileName, _ := cmd.Flags().GetString("profile")
		if profileName == "" {
			profileName = "Default"
		}

		// Step 1: Load latest knowledge items
		items, err := loadLatestKnowledgeItems()
		if err != nil {
			return fmt.Errorf("no ingested data — run 'pantheon seshat ingest' first: %w", err)
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
	seshatCmd.AddCommand(seshatAdaptersCmd)
	seshatCmd.AddCommand(seshatProfilesCmd)
	seshatCmd.AddCommand(seshatOpenCmd)
	seshatCmd.AddCommand(seshatSyncCmd)
	seshatCmd.AddCommand(seshatMcpCmd)
	seshatCmd.AddCommand(seshatAuthCmd)
}

// Storage helpers for ingested knowledge items

func seshatStoreDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "seshat", "store")
}

func saveKnowledgeItems(items []seshat.KnowledgeItem) (string, error) {
	dir := seshatStoreDir()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return "", err
	}

	path := filepath.Join(dir, fmt.Sprintf("ingestion_%s.json", time.Now().Format("20060102_150405")))
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}

	// Also write a "latest" symlink
	latestPath := filepath.Join(dir, "latest.json")
	os.Remove(latestPath) // remove old symlink
	if err := os.WriteFile(latestPath, data, 0644); err != nil {
		return "", err
	}

	return path, nil
}

func loadLatestKnowledgeItems() ([]seshat.KnowledgeItem, error) {
	path := filepath.Join(seshatStoreDir(), "latest.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var items []seshat.KnowledgeItem
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}
