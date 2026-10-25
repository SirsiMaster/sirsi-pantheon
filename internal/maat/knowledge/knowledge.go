// Package knowledge projects the retained local knowledge cache through Ma'at.
// It is deliberately read-only: Seshat remains the compatibility ingestion
// adapter while Ma'at owns the operator and agent-facing decision surface.
package knowledge

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

// System One is the public knowledge boundary.  The compatibility filter
// catches known credentials; these additional conservative rules prevent a
// local account/claim identifier or a standalone high-entropy credential from
// becoming visible simply because an older source adapter did not label it.
// A false positive withholds one item and leaves the retained cache untouched;
// a false negative would disclose local data to every Ma'at consumer.
var (
	longNumericIdentifier = regexp.MustCompile(`\b[0-9]{12,19}\b`)
	labeledIdentifier     = regexp.MustCompile(`(?i)\b(?:account|service|claim|policy|routing|member|customer|tax)\s*(?:number|no\.?|id|#)?\s*[:#]?\s*[0-9][0-9 -]{5,}[0-9]`)
	graphicToken          = regexp.MustCompile(`[[:graph:]]+`)
)

// Item is the sensitivity-filtered public portion of a retained knowledge
// record. The projection intentionally omits ingestion internals and source
// parser metadata so every consumer receives the same stable view.
type Item struct {
	ID             string               `json:"id,omitempty"`
	Classification string               `json:"classification,omitempty"`
	Resolution     ResolutionState      `json:"resolution,omitempty"`
	Title          string               `json:"title"`
	Summary        string               `json:"summary"`
	References     []seshat.KIReference `json:"references"`
	Origins        []Origin             `json:"origins,omitempty"`
	Revision       int                  `json:"revision,omitempty"`
	Excerpt        string               `json:"excerpt,omitempty"`
	WithheldReason string               `json:"withheld_reason,omitempty"`
}

// View is Ma'at's shared, read-only local knowledge contract.
type View struct {
	Items      []Item      `json:"items"`
	Total      int         `json:"total"`
	Withheld   int         `json:"withheld"`
	Coverage   Coverage    `json:"coverage"`
	NextCursor string      `json:"next_cursor,omitempty"`
	State      ImportState `json:"state,omitempty"`
}

// Load reads Ma'at's canonical local archive under home. It never migrates or
// ingests legacy files implicitly; malformed or unreadable archives fail closed.
func Load(home, query string) (View, error) {
	if strings.TrimSpace(home) == "" {
		return View{}, errors.New("Ma'at knowledge home is empty")
	}
	return LoadPage(home, SearchQuery{Text: query})
}

// LoadPage reads the shared archive projection. It never ingests sources or
// falls back to the legacy cache when the archive is corrupt or unavailable.
func LoadPage(home string, query SearchQuery) (View, error) {
	return LoadPageContext(context.Background(), home, query)
}

func LoadPageContext(ctx context.Context, home string, query SearchQuery) (View, error) {
	archive, err := Open(home)
	if err != nil {
		return View{}, err
	}
	defer archive.Close()
	page, err := archive.Search(ctx, query)
	if err != nil {
		return View{}, err
	}
	view := View{Items: make([]Item, 0, len(page.Items)), Total: page.Total, Withheld: page.Withheld, Coverage: page.Coverage, NextCursor: page.NextCursor}
	for _, record := range page.Items {
		view.Items = append(view.Items, Item{
			ID: record.ID, Classification: record.Classification, Resolution: record.Resolution,
			Title: record.Title, Summary: record.Summary, References: append([]seshat.KIReference(nil), record.References...),
			Origins: append([]Origin(nil), record.Origins...), Revision: record.Revision, Excerpt: record.Excerpt,
			WithheldReason: record.WithheldReason,
		})
	}
	for _, run := range page.Coverage.Sources {
		if run.State != ImportComplete && run.State != ImportNoNewItems {
			view.State = run.State
			break
		}
	}
	return view, nil
}

// Project returns the safe, query-filtered Ma'at view from an already loaded
// legacy record list. It is exported for deterministic tests and for callers
// that already hold an in-memory cache snapshot.
func Project(items []seshat.KnowledgeItem, query string) View {
	filter := seshat.DefaultFilter()
	needle := strings.ToLower(strings.TrimSpace(query))
	view := View{Items: make([]Item, 0, len(items))}
	for _, item := range items {
		content := item.Title + "\n" + item.Summary + "\n" + item.Content + "\n" + referencesText(item.References)
		if len(filter.Scan(content)) > 0 || hasSystemOneSensitiveContent(content) {
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

func hasSystemOneSensitiveContent(content string) bool {
	if longNumericIdentifier.MatchString(content) || labeledIdentifier.MatchString(content) {
		return true
	}
	for _, token := range graphicToken.FindAllString(content, -1) {
		if looksLikeStandaloneCredential(token) {
			return true
		}
	}
	return false
}

func looksLikeStandaloneCredential(token string) bool {
	if len(token) < 12 || len(token) > 128 || strings.Contains(token, "://") {
		return false
	}
	var upper, lower, digit, symbol bool
	for _, r := range token {
		switch {
		case unicode.IsUpper(r):
			upper = true
		case unicode.IsLower(r):
			lower = true
		case unicode.IsDigit(r):
			digit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			symbol = true
		}
	}
	return upper && lower && digit && symbol
}

func referencesText(refs []seshat.KIReference) string {
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		parts = append(parts, ref.Type+":"+ref.Value)
	}
	return strings.Join(parts, "\n")
}
