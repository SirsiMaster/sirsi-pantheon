package knowledge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestArchiveLiteralSearch(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	archive, err := Open(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := archive.Close(); err != nil {
			t.Error(err)
		}
	})
	raw, err := os.ReadFile("testdata/literal-search.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Records []string
		Cases   []struct {
			Name, Query string
			Want        []int
		}
		Invalid []string
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	records := make([]seshat.SourceRecord, len(fixture.Records))
	for i, body := range fixture.Records {
		records[i] = seshat.SourceRecord{Source: "claude", InstanceID: "literal-fixture", NativeItemID: fmt.Sprint(i), Schema: "fixture-v1", Title: fmt.Sprintf("Literal fixture %02d", i), Summary: "Retained reference", Content: body}
	}
	if _, err := archive.ImportSource(context.Background(), "claude", records, nil); err != nil {
		t.Fatal(err)
	}

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			page, err := archive.Search(context.Background(), SearchQuery{Text: tc.Query, Source: "claude"})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(page.Items))
			for _, item := range page.Items {
				got = append(got, item.Title)
			}
			want := make([]string, 0, len(tc.Want))
			for _, index := range tc.Want {
				want = append(want, fmt.Sprintf("Literal fixture %02d", index))
			}
			sort.Strings(got)
			sort.Strings(want)
			if page.Total != len(want) || !reflect.DeepEqual(got, want) {
				t.Fatalf("literal %q total=%d got=%v want=%v", tc.Query, page.Total, got, want)
			}
		})
	}
	for i, query := range fixture.Invalid {
		t.Run(fmt.Sprintf("invalid_%d", i), func(t *testing.T) {
			_, err := archive.Search(context.Background(), SearchQuery{Text: query})
			if !errors.Is(err, ErrInvalidSearchQuery) {
				t.Fatalf("query %q: %v", query, err)
			}
		})
	}
}

func TestArchiveLiteralSearchCursorCountsAndPrivacy(t *testing.T) {
	archive, _ := openTestArchive(t)
	ctx := context.Background()
	first := archiveRecord("claude", "literal", "first", "archive-drift one")
	second := archiveRecord("claude", "literal", "second", "archive-drift two")
	secret := archiveRecord("claude", "literal", "secret", "archive-drift token=never-persist-literal-secret")
	if _, err := archive.ImportSource(ctx, "claude", []seshat.SourceRecord{first, second, secret}, nil); err != nil {
		t.Fatal(err)
	}
	q := SearchQuery{Text: "archive-drift", Source: "claude", Limit: 1}
	page, err := archive.Search(ctx, q)
	if err != nil || page.Total != 2 || page.Withheld != 1 || len(page.Items) != 1 || page.NextCursor == "" || !strings.Contains(page.Items[0].Excerpt, "[") {
		t.Fatalf("first literal page=%+v %v", page, err)
	}
	q.Cursor = page.NextCursor
	next, err := archive.Search(ctx, q)
	if err != nil || next.Total != 2 || len(next.Items) != 1 || next.Items[0].ID == page.Items[0].ID || next.NextCursor != "" {
		t.Fatalf("next literal page=%+v %v", next, err)
	}
	// Equivalent tokenizer terms still do not authorize reusing another raw query's cursor.
	q.Text = "archive:drift"
	if _, err := archive.Search(ctx, q); err == nil {
		t.Fatal("changed raw query accepted bound cursor")
	}
	q.Text = "archive-drift"
	if _, err := archive.ImportSource(ctx, "claude", []seshat.SourceRecord{archiveRecord("claude", "literal", "third", "archive-drift three")}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Search(ctx, q); err == nil {
		t.Fatal("changed generation accepted old literal cursor")
	}
}

func TestArchiveLiteralSearchInvalidBeforeOpen(t *testing.T) {
	for _, query := range []string{"---", strings.Repeat("a", 513), "a\x00b", string([]byte{0xff})} {
		home := filepath.Join(t.TempDir(), "absent-home")
		if _, err := LoadPageContext(context.Background(), home, SearchQuery{Text: query}); !errors.Is(err, ErrInvalidSearchQuery) {
			t.Fatalf("invalid query error=%v", err)
		}
		if _, err := os.Lstat(home); !os.IsNotExist(err) {
			t.Fatalf("invalid query touched archive home: %v", err)
		}
	}
}

// Synthetic raw-query diagnostic binds the observed hyphen failure to the old
// SQL expression path, without exposing user content or SQLite errors in the API.
func TestArchiveLiteralSearchRawFTSDiagnostic(t *testing.T) {
	archive, _ := openTestArchive(t)
	item := archiveRecord("claude", "literal", "raw-diagnostic", "archive-drift witness")
	if _, err := archive.ImportSource(context.Background(), "claude", []seshat.SourceRecord{item}, nil); err != nil {
		t.Fatal(err)
	}
	var count int
	err := archive.db.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM knowledge_fts WHERE knowledge_fts MATCH ?`, "archive-drift").Scan(&count)
	if err == nil {
		t.Fatal("raw FTS hyphen diagnostic unexpectedly accepted operator syntax")
	}
	t.Logf("synthetic raw FTS diagnostic (%T): %v", err, err)
	page, err := archive.Search(context.Background(), SearchQuery{Text: "archive-drift"})
	if err != nil || page.Total != 1 {
		t.Fatalf("literal replacement=%+v %v", page, err)
	}
}
