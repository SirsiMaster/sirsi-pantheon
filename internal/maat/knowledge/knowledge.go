// Package knowledge projects the retained local knowledge cache through Ma'at.
// It is deliberately read-only: Seshat remains the compatibility ingestion
// adapter while Ma'at owns the operator and agent-facing decision surface.
package knowledge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

// Item is the sensitivity-filtered public portion of a retained knowledge
// record. The projection intentionally omits ingestion internals and source
// parser metadata so every consumer receives the same stable view.
type Item struct {
	Title      string               `json:"title"`
	Summary    string               `json:"summary"`
	References []seshat.KIReference `json:"references"`
}

// View is Ma'at's shared, read-only local knowledge contract.
type View struct {
	Items    []Item `json:"items"`
	Total    int    `json:"total"`
	Withheld int    `json:"withheld"`
}

// Load reads Ma'at's compatibility cache under home. A missing cache is an
// honest empty library; malformed or unreadable cache data is rejected rather
// than being presented as complete knowledge.
func Load(home, query string) (View, error) {
	if strings.TrimSpace(home) == "" {
		return View{}, errors.New("Ma'at knowledge home is empty")
	}
	path := filepath.Join(home, ".config", "seshat", "store", "latest.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyView(), nil
	}
	if err != nil {
		return View{}, fmt.Errorf("read Ma'at knowledge cache: %w", err)
	}
	var items []seshat.KnowledgeItem
	if err := json.Unmarshal(data, &items); err != nil {
		return View{}, fmt.Errorf("decode Ma'at knowledge cache: %w", err)
	}
	return Project(items, query), nil
}

// Project returns the safe, query-filtered Ma'at view from an already loaded
// legacy record list. It is exported for deterministic tests and for callers
// that already hold an in-memory cache snapshot.
func Project(items []seshat.KnowledgeItem, query string) View {
	filter := seshat.DefaultFilter()
	needle := strings.ToLower(strings.TrimSpace(query))
	view := View{Items: make([]Item, 0, len(items))}
	for _, item := range items {
		content := item.Title + "\n" + item.Summary + "\n" + referencesText(item.References)
		if len(filter.Scan(content)) > 0 {
			view.Withheld++
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(content), needle) {
			continue
		}
		view.Items = append(view.Items, Item{
			Title:      item.Title,
			Summary:    item.Summary,
			References: append([]seshat.KIReference(nil), item.References...),
		})
	}
	view.Total = len(view.Items)
	return view
}

func emptyView() View {
	return View{Items: []Item{}}
}

func referencesText(refs []seshat.KIReference) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, ref.Type+":"+ref.Value)
	}
	return strings.Join(parts, "\n")
}
