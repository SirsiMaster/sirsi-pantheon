package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

func dashboardHistoryJournal(t *testing.T) *maat.FileDecisionJournal {
	t.Helper()
	root := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "decisions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 0; i < 2012; i++ {
		row := maat.Decision{Time: time.Date(2026, 9, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339Nano), Host: "inert", Kind: "allocation", Requester: "fixture", Assessed: "read only", Determination: "deny", Why: strings.Repeat("x", 3000), Evidence: fmt.Sprintf("new:%d", i)}
		if i == 0 {
			row.Kind = "diagnostic owner review"
			row.Why = "retained-link-target"
			row.Evidence = "review:old"
		}
		if i == 1 {
			row.Kind = "diagnostic owner acceptance"
			row.Why = "accepted retained conclusion"
			row.Determination = "accepted"
			row.Evidence = "accept:old"
			row.ResolutionFor = "review:old"
		}
		if err = enc.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
	return &maat.FileDecisionJournal{Path: path}
}
func TestMaatHistoryHTTPBoundedOlderLinkedMatch(t *testing.T) {
	j := dashboardHistoryJournal(t)
	s := New(Config{MaatHistoryFn: func(ctx context.Context, q casebook.HistoryQuery) (casebook.HistorySearchResult, error) {
		q.Context = ctx
		return casebook.SearchHistory(j, q)
	}})
	query := "/api/maat/casebook?q=accepted+retained+conclusion&kind=diagnostic+owner+review&status=resolved&limit=1"
	cursor := ""
	snapshot := ""
	partial := false
	for n := 0; n < 80; n++ {
		url := query
		if cursor != "" {
			url += "&cursor=" + cursor
		}
		rec := httptest.NewRecorder()
		s.apiMaatCasebook(rec, httptest.NewRequest(http.MethodGet, url, nil))
		if rec.Code != 200 {
			t.Fatalf("history status=%d body=%s", rec.Code, rec.Body.String())
		}
		var page struct {
			casebook.View
			History casebook.HistoryWindow `json:"history"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if snapshot == "" {
			snapshot = page.History.Snapshot
		} else if snapshot != page.History.Snapshot {
			t.Fatal("snapshot drift within continuation")
		}
		if page.History.Coverage == "complete" {
			if !partial || len(page.Cases) != 1 || page.Cases[0].Evidence != "review:old" || page.Cases[0].Resolution != "accepted retained conclusion" || page.Cases[0].Status != casebook.StatusResolved {
				t.Fatalf("old linked result=%+v", page)
			}
			return
		}
		partial = true
		if len(page.Cases) != 0 || page.History.NextCursor == "" {
			t.Fatalf("provisional result=%+v", page)
		}
		cursor = page.History.NextCursor
	}
	t.Fatal("history continuation exhausted finite fixture budget")
}
func TestMaatHistoryHTTPRejectsBeforeProducerAndHonorsCancellation(t *testing.T) {
	reads := 0
	s := New(Config{MaatHistoryFn: func(ctx context.Context, q casebook.HistoryQuery) (casebook.HistorySearchResult, error) {
		reads++
		return casebook.HistorySearchResult{}, ctx.Err()
	}})
	for _, query := range []string{"limit=0", "limit=1&limit=2", "q=a&query=b", "scope=bad", "cursor=bad", "status=unknown", "future=ignored"} {
		rec := httptest.NewRecorder()
		s.apiMaatCasebook(rec, httptest.NewRequest(http.MethodGet, "/api/maat/casebook?"+query, nil))
		if rec.Code != 400 {
			t.Fatalf("%s status=%d", query, rec.Code)
		}
	}
	if reads != 0 {
		t.Fatalf("malformed input read %d times", reads)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := httptest.NewRecorder()
	s.apiMaatCasebook(rec, httptest.NewRequest(http.MethodGet, "/api/maat/casebook", nil).WithContext(ctx))
	if rec.Code != 408 || !strings.Contains(rec.Body.String(), "read_cancelled") {
		t.Fatalf("cancellation=%d %s", rec.Code, rec.Body.String())
	}
}
func TestMaatHistoryHTTPSnapshotChangeIsNotCompleteEmpty(t *testing.T) {
	s := New(Config{MaatHistoryFn: func(context.Context, casebook.HistoryQuery) (casebook.HistorySearchResult, error) {
		return casebook.HistorySearchResult{}, maat.ErrHistorySnapshotChanged
	}})
	rec := httptest.NewRecorder()
	s.apiMaatCasebook(rec, httptest.NewRequest(http.MethodGet, "/api/maat/casebook", nil))
	if rec.Code != 409 || !strings.Contains(rec.Body.String(), "snapshot_changed") || strings.Contains(rec.Body.String(), `"cases":[]`) {
		t.Fatalf("drift disguised as empty: %d %s", rec.Code, rec.Body.String())
	}
}
func TestMaatKnowledgeHTTPReadModesAndMalformedRefusal(t *testing.T) {
	var selected MaatKnowledgeReadRequest
	reads := 0
	s := New(Config{MaatKnowledgeReadFn: func(ctx context.Context, r MaatKnowledgeReadRequest) (any, error) {
		reads++
		selected = r
		return map[string]any{"items": []any{}, "total": 0}, ctx.Err()
	}})
	id := strings.Repeat("a", 64)
	for _, mode := range []string{"search", "detail", "history"} {
		query := "?mode=" + mode
		if mode != "search" {
			query += "&id=" + id
		}
		rec := httptest.NewRecorder()
		s.apiMaatKnowledge(rec, httptest.NewRequest(http.MethodGet, "/api/maat/knowledge"+query, nil))
		if rec.Code != 200 || selected.Mode != mode {
			t.Fatalf("mode=%s response=%d", mode, rec.Code)
		}
	}
	before := reads
	for _, query := range []string{"mode=write", "mode=detail&id=bad", "mode=detail&id=" + id + "&q=discarded", "history=1", "revision=-1", "cursor=e30", "source=claude&source=other"} {
		rec := httptest.NewRecorder()
		s.apiMaatKnowledge(rec, httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?"+query, nil))
		if rec.Code != 400 {
			t.Fatalf("invalid query accepted=%s status=%d", query, rec.Code)
		}
	}
	if reads != before {
		t.Fatal("malformed archive query reached producer")
	}
}
