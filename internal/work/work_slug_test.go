package work

import (
	"strings"
	"testing"
)

// Router item 20260730-225729: slug truncation at 60 chars could cut mid-word
// and leave a trailing hyphen, so `router send` printed an id the store did not
// have — breaking any consumer that pins the printed id.
func TestSlugNeverEndsWithHyphenAfterTruncation(t *testing.T) {
	// Crafted so byte 60 lands right after a hyphen.
	title := strings.Repeat("ab-", 25) // "ab-ab-…" — position 60 is a '-'
	got := slugify(title)
	if strings.HasSuffix(got, "-") {
		t.Fatalf("slug %q ends with a hyphen — the printed id will not match the stored id", got)
	}
	if len(got) > 60 {
		t.Fatalf("slug %q exceeds the 60-char bound", got)
	}
}
