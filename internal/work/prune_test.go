package work

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeItem drops a raw item file with the given status/closed frontmatter.
func writeItem(t *testing.T, root, id, status, closed string) {
	t.Helper()
	closedLine := ""
	if closed != "" {
		closedLine = "closed: " + closed + "\n"
	}
	body := "body\n"
	if id == "20260101-old-closed" {
		body = strings.Repeat("payload line with enough retained context to compact\n", 64)
	}
	content := "---\nfrom: a\nto: b\ntitle: t\nstatus: " + status + "\nopened: 2026-01-01T00:00:00Z\n" + closedLine + "---\n\n## Instructions\n\n" + body
	if err := os.WriteFile(filepath.Join(itemsDir(root), id+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPruneItems(t *testing.T) {
	root := t.TempDir()
	if err := EnsureRoot(root); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -120).UTC().Format(time.RFC3339)
	recent := time.Now().AddDate(0, 0, -10).UTC().Format(time.RFC3339)

	writeItem(t, root, "20260101-old-closed", "closed", old)       // prune
	writeItem(t, root, "20260601-recent-closed", "closed", recent) // keep (within window)
	writeItem(t, root, "20260101-open-old", "open", "")            // keep (open, never pruned)
	writeItem(t, root, "20260101-closed-nodate", "closed", "")     // keep (unparseable close date)

	cutoff := time.Now().AddDate(0, 0, -90)

	// Dry-run reports the one candidate but deletes nothing.
	dry, err := PruneItems(root, cutoff, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry) != 1 || dry[0].ID != "20260101-old-closed" {
		t.Fatalf("dry-run: want 1 (old-closed), got %+v", dry)
	}
	if _, statErr := os.Stat(filepath.Join(itemsDir(root), "20260101-old-closed.md")); statErr != nil {
		t.Fatalf("dry-run must not delete: %v", statErr)
	}
	if dry[0].Bytes == 0 {
		t.Fatalf("dry-run should report byte size")
	}

	// Real run tombstones exactly the old closed item.
	got, err := PruneItems(root, cutoff, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1 pruned, got %d", len(got))
	}
	if got[0].After <= 0 || got[0].After >= got[0].Bytes {
		t.Fatalf("tombstone should reclaim bytes, got before=%d after=%d", got[0].Bytes, got[0].After)
	}
	tombstone, err := os.ReadFile(filepath.Join(itemsDir(root), "20260101-old-closed.md"))
	if err != nil {
		t.Fatalf("old closed item id/file must be retained: %v", err)
	}
	text := string(tombstone)
	for _, want := range []string{
		"tombstoned: true",
		"content_hash_sha256:",
		"status: closed",
		"from: a",
		"to: b",
		"Payload compacted by router retention",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("tombstone missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\nbody\n") {
		t.Fatalf("payload body should be compacted, got:\n%s", text)
	}
	for _, keep := range []string{"20260601-recent-closed", "20260101-open-old", "20260101-closed-nodate"} {
		if _, statErr := os.Stat(filepath.Join(itemsDir(root), keep+".md")); statErr != nil {
			t.Fatalf("%s must be kept: %v", keep, statErr)
		}
	}

	again, err := PruneItems(root, cutoff, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("already-tombstoned item must not be compacted again: %+v", again)
	}
}

func TestPruneItemsEmptyRoot(t *testing.T) {
	root := t.TempDir()
	got, err := PruneItems(root, time.Now(), false)
	if err != nil {
		t.Fatalf("empty root should not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("empty root should prune nothing")
	}
}
