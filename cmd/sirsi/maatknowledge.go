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
	maatCmd.AddCommand(maatKnowledgeCmd)
}
