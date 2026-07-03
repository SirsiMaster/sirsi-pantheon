package routerstore

import (
	"regexp"
	"strings"
)

// slugRe matches runs of non-alphanumeric characters. Kept identical to
// internal/work's slug rule so a Send here produces the same id the file
// router would, keeping the store and the filesystem in agreement.
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugify lower-cases, replaces non-alphanumeric runs with '-', trims dashes,
// caps length at 60, and falls back to "untitled" for empty input. It mirrors
// internal/work.slugify byte-for-byte.
func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "untitled"
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}
