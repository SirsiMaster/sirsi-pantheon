package knowledge

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ErrInvalidSearchQuery is safe to show on every read surface. Search accepts
// literal text, not the SQLite FTS expression language. Empty text still browses.
var ErrInvalidSearchQuery = errors.New("invalid_search_query: enter up to 512 bytes of literal words containing a letter or number; advanced search operators are not supported")

// ValidateSearchText checks the same grammar used by Search, before a caller
// opens the private archive. Whitespace-separated terms retain AND semantics.
func ValidateSearchText(text string) error {
	_, err := literalSearchExpression(text)
	return err
}

func literalSearchExpression(text string) (string, error) {
	if len(text) > 512 || !utf8.ValidString(text) {
		return "", ErrInvalidSearchQuery
	}
	for _, r := range text {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return "", ErrInvalidSearchQuery
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", nil
	}
	terms := make([]string, 0)
	for _, term := range strings.Fields(text) {
		if !strings.ContainsFunc(term, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsNumber(r) }) {
			continue
		}
		// FTS5 quoted strings escape double quotes by doubling them. All user
		// punctuation/operators stay inside a string; only this code supplies AND.
		terms = append(terms, `"`+strings.ReplaceAll(term, `"`, `""`)+`"`)
	}
	if len(terms) == 0 {
		return "", ErrInvalidSearchQuery
	}
	return strings.Join(terms, " AND "), nil
}
