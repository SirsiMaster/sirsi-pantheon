package knowledge

import (
	"context"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestArchiveSearchSnippetUsesDirectFTSCursor(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "fixture-profile", "snippet-cursor", "alpha needle omega")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	page, err := archive.Search(context.Background(), SearchQuery{Text: "needle"})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("search page = %+v, %v", page, err)
	}
	if !strings.Contains(page.Items[0].Excerpt, "[needle]") {
		t.Fatalf("direct FTS excerpt = %q", page.Items[0].Excerpt)
	}
}

// This fixture-only diagnostic records SQLite's exact error for the previous
// windowed FTS auxiliary-function shape. Its generated content is non-sensitive;
// production Search continues to return only its stable sanitized errors.
func TestArchiveSearchWindowedSnippetDiagnostic(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "fixture-profile", "snippet-diagnostic", "diagnostic needle phrase")
	if _, err := archive.ImportSource(context.Background(), item.Source, []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	const previousQuery = `SELECT id,revision,excerpt FROM (
		SELECT r.id,v.revision,snippet(knowledge_fts,4,'[',']','…',18) AS excerpt,v.content_digest,
		ROW_NUMBER() OVER(PARTITION BY v.content_digest ORDER BY r.id ASC,v.revision DESC) AS duplicate_rank
		FROM records r JOIN versions v ON v.record_id=r.id
		JOIN knowledge_fts ON knowledge_fts.record_id=r.id AND knowledge_fts.revision=v.revision
		WHERE r.withheld_reason='' AND (?='' OR r.classification=?) AND (?='' OR r.resolution=?)
		AND (?='' OR r.source=?) AND knowledge_fts MATCH ? AND v.revision=r.current_revision
	) WHERE duplicate_rank=1 ORDER BY id LIMIT 2 OFFSET 0`
	var id, excerpt string
	var revision int
	err := archive.db.QueryRowContext(context.Background(), previousQuery,
		"", "", "", "", "", "", "needle").Scan(&id, &revision, &excerpt)
	if err != nil {
		t.Logf("fixture-only previous FTS/window SQL diagnostic (%T): %v", err, err)
	} else {
		t.Logf("fixture-only previous FTS/window SQL returned a row; SQLite did not reproduce an auxiliary-context error")
	}
}
