package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The panel shows each release's headline items and marks the unreleased section;
// a bullet with a bold lead keeps the lead (both shapes).
func TestReadChangelogReleases(t *testing.T) {
	p := filepath.Join(t.TempDir(), "CHANGELOG.md")
	body := "# Changelog\n\n## [Unreleased] — wip\n\n- **Alpha thing.** It does a\n  long explanation.\n- plain bullet\n\n## [0.24.68] — 2026-10-05\n\n- **Beta thing:** shipped.\n\n## [0.24.67] — 2026-10-04\n\n- old\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readChangelogReleases(p, 1)
	if len(got) != 2 || got[0].Version != "Unreleased" || got[0].Date != "" || got[1].Version != "0.24.68" || got[1].Date != "2026-10-05" {
		t.Fatalf("sections: %+v", got)
	}
	if len(got[0].Items) != 2 || got[0].Items[1] != "plain bullet" || got[0].Items[0][:11] != "Alpha thing" {
		t.Fatalf("unreleased items: %+v", got[0].Items)
	}
	if readChangelogReleases(filepath.Join(t.TempDir(), "missing.md"), 3) != nil {
		t.Fatal("a missing changelog must yield nothing, not an invented list")
	}
}
