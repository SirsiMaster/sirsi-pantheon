package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

// maatKnowledgeItem is Ma'at's read-only projection of the retained local
// knowledge cache. Seshat remains the ingestion compatibility layer for now,
// but Ma'at is the single operator authority for inspecting the evidence and
// knowledge that informs decisions.
type maatKnowledgeItem struct {
	Title      string               `json:"title"`
	Summary    string               `json:"summary"`
	References []seshat.KIReference `json:"references"`
}

type maatKnowledgeView struct {
	Items    []maatKnowledgeItem `json:"items"`
	Total    int                 `json:"total"`
	Withheld int                 `json:"withheld"`
}

var maatKnowledgeCmd = &cobra.Command{
	Use:   "knowledge [text]",
	Short: "Inspect Ma'at's local evidence and knowledge library",
	Long: `Inspect the local knowledge that Ma'at can connect to evidence-backed decisions.

This is a read-only projection. It does not ingest, export, rescore, or change
the local cache. The legacy 'seshat' commands remain available for ingestion
compatibility while Ma'at is the canonical operator surface.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		items, err := loadLatestKnowledgeItems()
		if err != nil {
			// An absent local cache is an honest empty library, not an error.
			items = []seshat.KnowledgeItem{}
		}
		needle := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
		items, withheld := safeMaatKnowledgeItems(items)
		view := maatKnowledgeView{Items: make([]maatKnowledgeItem, 0, len(items)), Withheld: withheld}
		for _, item := range items {
			if needle != "" && !strings.Contains(strings.ToLower(strings.Join([]string{item.Title, item.Summary, referencesText(item.References)}, "\n")), needle) {
				continue
			}
			view.Items = append(view.Items, maatKnowledgeItem{Title: item.Title, Summary: item.Summary, References: item.References})
		}
		view.Total = len(view.Items)

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
		return seshatIngestCmd.RunE(seshatIngestCmd, args)
	},
}

func referencesText(refs []seshat.KIReference) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, ref.Type+":"+ref.Value)
	}
	return strings.Join(parts, "\n")
}

// safeMaatKnowledgeItems makes the operator view fail closed for a legacy
// cache. Old Seshat ingestions may predate source filtering, so Ma'at never
// renders an item with a secret match in title, summary, or reference metadata.
// The cache is not mutated: repair and re-ingestion remain an explicit action.
func safeMaatKnowledgeItems(items []seshat.KnowledgeItem) ([]seshat.KnowledgeItem, int) {
	filter := seshat.DefaultFilter()
	safe := make([]seshat.KnowledgeItem, 0, len(items))
	withheld := 0
	for _, item := range items {
		content := item.Title + "\n" + item.Summary + "\n" + referencesText(item.References)
		if len(filter.Scan(content)) > 0 {
			withheld++
			continue
		}
		safe = append(safe, item)
	}
	return safe, withheld
}

func init() {
	maatKnowledgeRefreshCmd.Flags().String("source", "", "Specific local source adapter to refresh")
	maatKnowledgeRefreshCmd.Flags().String("since", "", "Refresh items since a duration or YYYY-MM-DD date")
	maatKnowledgeRefreshCmd.Flags().String("profile", "", "Chrome profile name or display name")
	maatKnowledgeRefreshCmd.Flags().Bool("all-profiles", false, "Refresh Chrome history from every local profile")
	maatKnowledgeCmd.AddCommand(maatKnowledgeRefreshCmd)
	maatCmd.AddCommand(maatKnowledgeCmd)
}
