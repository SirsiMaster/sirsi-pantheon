package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/output"
)

var maatKnowledgeCmd = &cobra.Command{
	Use:   "knowledge [text]",
	Short: "Inspect Ma'at's local evidence and knowledge library",
	Long: `Inspect the local knowledge that Ma'at can connect to evidence-backed decisions.

This is a read-only projection. It does not ingest, export, rescore, or change
the local cache. The legacy 'seshat' commands remain available for ingestion
compatibility while Ma'at is the canonical operator surface.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("locate Ma'at knowledge home: %w", err)
		}
		view, err := knowledge.Load(home, strings.Join(args, " "))
		if err != nil {
			return err
		}

		if JsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(view)
		}
		if view.Total == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "Ma'at knowledge is empty. Ingest a source through the compatibility commands to populate the local library.")
			return nil
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Ma'at knowledge — %d item(s)\n", view.Total)
		for _, item := range view.Items {
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n%s\n", item.Title, item.Summary)
		}
		return nil
	},
}

// maatKnowledgeRefreshCmd is the public write path for refreshing the local
// knowledge cache. Its implementation deliberately delegates to the retained
// Seshat ingestion adapter rather than duplicating source adapters or their
// filtering semantics. It exposes only refresh-scoped inputs: exports stay
// outside Ma'at's local knowledge refresh contract.
var maatKnowledgeRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh Ma'at's local knowledge from configured sources",
	Long: `Refresh Ma'at's retained local knowledge through the compatibility ingestion adapter.

This can read configured local sources and update the local cache. It does not
export knowledge, open a browser, authorize work, or make a remote decision.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		started := time.Now()
		// The legacy adapter owns source parsing and cache writes. Copy only the
		// explicit, bounded refresh flags into that adapter; `--export` is not
		// exposed here, so a Ma'at refresh cannot become an external transfer.
		for _, name := range []string{"source", "since", "profile", "all-profiles"} {
			flag := cmd.Flags().Lookup(name)
			if flag == nil {
				return fmt.Errorf("Ma'at knowledge refresh: missing %s flag contract", name)
			}
			if err := seshatIngestCmd.Flags().Set(name, flag.Value.String()); err != nil {
				return fmt.Errorf("Ma'at knowledge refresh: set %s: %w", name, err)
			}
		}
		if err := maatKnowledgeRefreshDelegate(seshatIngestCmd, args); err != nil {
			return err
		}
		maatKnowledgeRefreshResult(time.Since(started)).Render()
		return nil
	},
}

// maatKnowledgeRefreshDelegate is deliberately narrow: the legacy adapter is
// still the sole parser and cache writer, while Ma'at owns the user-facing
// completion receipt. Keeping this seam lets the native surface distinguish a
// completed refresh from an unstructured adapter transcript.
var maatKnowledgeRefreshDelegate = func(cmd *cobra.Command, args []string) error {
	return seshatIngestCmd.RunE(cmd, args)
}

func maatKnowledgeRefreshResult(elapsed time.Duration) *output.CommandResult {
	return &output.CommandResult{
		Command:    "sirsi maat knowledge refresh",
		BriefTitle: "Ma'at knowledge refresh",
		Summary:    "Ma'at refreshed the local knowledge cache. Review the retained items before using them in a decision.",
		Status:     "ok",
		Duration:   elapsed,
		NextActions: []output.NextAction{{
			Label:       "Open Ma'at knowledge",
			Command:     "sirsi maat knowledge --json",
			Description: "Inspect the retained, sensitivity-filtered knowledge projection.",
		}},
	}
}

func init() {
	maatKnowledgeRefreshCmd.Flags().String("source", "", "Specific local source adapter to refresh")
	maatKnowledgeRefreshCmd.Flags().String("since", "", "Refresh items since a duration or YYYY-MM-DD date")
	maatKnowledgeRefreshCmd.Flags().String("profile", "", "Chrome profile name or display name")
	maatKnowledgeRefreshCmd.Flags().Bool("all-profiles", false, "Refresh Chrome history from every local profile")
	maatKnowledgeCmd.AddCommand(maatKnowledgeRefreshCmd)
	maatCmd.AddCommand(maatKnowledgeCmd)
}
