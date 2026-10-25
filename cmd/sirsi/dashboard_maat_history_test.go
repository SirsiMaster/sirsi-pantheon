package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/dashboard"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestDashboardCanonicalHistoryProducerBeforeWindow(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	journal, err := maat.NewDefaultDecisionJournal()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(journal.Path), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(journal.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i := 0; i < 2012; i++ {
		row := maat.Decision{Time: time.Date(2026, 9, 1, 0, i, 0, 0, time.UTC).Format(time.RFC3339Nano), Host: "fixture", Kind: "allocation", Requester: "inert", Assessed: "fixture", Determination: "deny", Why: "noise", Evidence: fmt.Sprintf("new:%d", i)}
		if i == 0 {
			row.Why = "dashboard-older-match"
			row.Evidence = "old:dashboard"
		}
		if err = enc.Encode(row); err != nil {
			t.Fatal(err)
		}
	}
	if err = f.Sync(); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	q := casebook.HistoryQuery{Query: casebook.Query{Text: "dashboard-older-match", Limit: 1}, Scope: maat.HistoryScopeAll}
	for n := 0; n < 80; n++ {
		page, err := collectDashboardMaatHistory(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		if page.Window.Coverage == "complete" {
			if len(page.View.Cases) != 1 || page.View.Cases[0].Evidence != "old:dashboard" {
				t.Fatalf("canonical producer truncated before filter=%+v", page)
			}
			return
		}
		if len(page.View.Cases) != 0 || page.Window.NextCursor == "" {
			t.Fatalf("provisional or stalled producer=%+v", page)
		}
		q.Cursor = page.Window.NextCursor
	}
	t.Fatal("bounded canonical producer did not finish fixture")
}
func TestDashboardCanonicalKnowledgeReadProducer(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	a, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	record := seshat.SourceRecord{Source: "claude", InstanceID: "inert", NativeItemID: "one", Schema: "fixture-v1", Title: "Retained", Content: "canonical retained content"}
	if _, err = a.ImportSource(context.Background(), "claude", []seshat.SourceRecord{record}, nil); err != nil {
		t.Fatal(err)
	}
	value, err := collectDashboardMaatKnowledgeRead(context.Background(), dashboard.MaatKnowledgeReadRequest{Mode: "search", Query: knowledge.SearchQuery{Text: "retained", Limit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	view, ok := value.(knowledge.View)
	if !ok || len(view.Items) != 1 {
		t.Fatalf("canonical search=%+v", value)
	}
	id := view.Items[0].ID
	for _, mode := range []string{"detail", "history"} {
		value, err = collectDashboardMaatKnowledgeRead(context.Background(), dashboard.MaatKnowledgeReadRequest{Mode: mode, ID: id, Query: knowledge.SearchQuery{Limit: 1}})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(value)
		if err != nil || !strings.Contains(string(raw), id) {
			t.Fatalf("canonical %s=%s %v", mode, raw, err)
		}
	}
}
