package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

var seshatCmd = &cobra.Command{
	Use:   "seshat",
	Short: "𓁆 Seshat — Gemini Bridge (knowledge sync)",
	Long: `𓁆 Seshat — The Scribe. Goddess of writing, wisdom, and measurement.

Bidirectional knowledge sync between Gemini AI Mode, NotebookLM, and Antigravity IDE.

Six directions:
  1. Gemini → NotebookLM    Extract conversations → package as sources
  2. NotebookLM → Gemini    Query insights → inject into GEMINI.md
  3. NotebookLM → Antigravity  Distill → inject as Knowledge Items
  4. Antigravity → NotebookLM  Export KIs → upload as sources
  5. Gemini → Antigravity    Extract → inject as Knowledge Items
  6. Antigravity → Gemini    Export KIs → GEMINI.md context`,
	Run: func(cmd *cobra.Command, args []string) {
		_ = cmd.Help()
	},
}

var seshatListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all Antigravity Knowledge Items",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := seshat.DefaultPaths()
		items, err := seshat.ListKnowledgeItems(paths)
		if err != nil {
			return fmt.Errorf("list KIs: %w", err)
		}

		if JsonOutput {
			for _, item := range items {
				fmt.Println(item)
			}
			return nil
		}

		fmt.Println("📚 Antigravity Knowledge Items:")
		fmt.Println()
		for i, item := range items {
			ki, err := seshat.ReadKnowledgeItem(paths, item)
			if err != nil {
				fmt.Printf("  %d. %s (⚠️ unreadable)\n", i+1, item)
				continue
			}
			fmt.Printf("  %d. %s\n", i+1, ki.Title)
			fmt.Printf("     %s\n", truncate(ki.Summary, 80))
			fmt.Println()
		}
		fmt.Printf("Total: %d Knowledge Items\n", len(items))
		return nil
	},
}

var seshatExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export Knowledge Items as NotebookLM-ready Markdown",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := seshat.DefaultPaths()
		outputDir, _ := cmd.Flags().GetString("output")
		if outputDir == "" {
			outputDir = "./seshat-export"
		}

		kiName, _ := cmd.Flags().GetString("ki")
		if kiName != "" {
			md, err := seshat.ExportKIToMarkdown(paths, kiName)
			if err != nil {
				return err
			}
			outFile := fmt.Sprintf("%s/ki_%s.md", outputDir, kiName)
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				return err
			}
			if err := os.WriteFile(outFile, []byte(md), 0644); err != nil {
				return err
			}
			fmt.Printf("📚 Exported: %s → %s\n", kiName, outFile)
			return nil
		}

		exported, err := seshat.ExportAllKIsToMarkdown(paths, outputDir)
		if err != nil {
			return err
		}
		fmt.Printf("\n🎉 Exported %d Knowledge Items to %s\n", len(exported), outputDir)
		return nil
	},
}

var seshatSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync Knowledge Items to GEMINI.md context",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := seshat.DefaultPaths()
		kiName, _ := cmd.Flags().GetString("ki")
		target, _ := cmd.Flags().GetString("target")

		if kiName == "" || target == "" {
			return fmt.Errorf("--ki and --target are required")
		}

		if err := seshat.SyncKIToGeminiMD(paths, kiName, target); err != nil {
			return err
		}

		fmt.Printf("✅ Synced KI '%s' → %s\n", kiName, target)
		return nil
	},
}

var seshatConversationsCmd = &cobra.Command{
	Use:   "conversations",
	Short: "List Antigravity brain conversations",
	RunE: func(cmd *cobra.Command, args []string) error {
		paths := seshat.DefaultPaths()
		lastN, _ := cmd.Flags().GetInt("last")
		if lastN == 0 {
			lastN = 10
		}

		ids, err := seshat.ListBrainConversations(paths, lastN)
		if err != nil {
			return err
		}

		fmt.Printf("💬 Last %d Antigravity Conversations:\n\n", len(ids))
		for i, id := range ids {
			fmt.Printf("  %d. %s\n", i+1, id)
		}
		return nil
	},
}

func init() {
	// Export flags
	seshatExportCmd.Flags().String("output", "./seshat-export", "Output directory")
	seshatExportCmd.Flags().String("ki", "", "Export a specific Knowledge Item")

	// Sync flags
	seshatSyncCmd.Flags().String("ki", "", "Knowledge Item name to sync")
	seshatSyncCmd.Flags().String("target", "", "Target GEMINI.md file path")

	// Conversations flags
	seshatConversationsCmd.Flags().Int("last", 10, "Show last N conversations")

	// Build command tree
	seshatCmd.AddCommand(seshatListCmd)
	seshatCmd.AddCommand(seshatExportCmd)
	seshatCmd.AddCommand(seshatSyncCmd)
	seshatCmd.AddCommand(seshatConversationsCmd)
}
