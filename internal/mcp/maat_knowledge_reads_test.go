package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func knowledgeResult(t *testing.T, args map[string]interface{}, into any) {
	t.Helper()
	result, err := handleMaatKnowledge(args)
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("knowledge read=%+v %v", result, err)
	}
	if err = json.Unmarshal([]byte(result.Content[0].Text), into); err != nil {
		t.Fatal(err)
	}
}
func TestMaatKnowledgeMCPCanonicalSearchDetailHistoryAndPrivacy(t *testing.T) {
	home := maatKnowledgeTestHome(t)
	a, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	record := seshat.SourceRecord{Source: "claude", InstanceID: "inert", NativeItemID: "one", Schema: "fixture-v1", Title: "Reference", Summary: "retained evidence", Content: "first complete evidence", References: []seshat.KIReference{{Type: "file", Value: "private-fixture.txt"}}}
	second := record
	second.NativeItemID = "two"
	second.Content = "second complete evidence"
	secret := record
	secret.NativeItemID = "token=never-persist-this-secret"
	secret.Title = "private"
	if _, err = a.ImportSource(context.Background(), "claude", []seshat.SourceRecord{record, second, secret}, nil); err != nil {
		t.Fatal(err)
	}
	record.Content = "revised complete evidence"
	if _, err = a.ImportSource(context.Background(), "claude", []seshat.SourceRecord{record}, nil); err != nil {
		t.Fatal(err)
	}
	old := maatKnowledgeHome
	t.Cleanup(func() { maatKnowledgeHome = old })
	maatKnowledgeHome = func() (string, error) { return home, nil }
	var page knowledge.View
	knowledgeResult(t, map[string]interface{}{"query": "evidence", "source": "claude", "limit": float64(1)}, &page)
	if len(page.Items) != 1 || page.Total != 2 || page.NextCursor == "" {
		t.Fatalf("canonical search=%+v", page)
	}
	first := page.Items[0]
	var detail knowledge.ArchiveRecord
	knowledgeResult(t, map[string]interface{}{"mode": "detail", "id": first.ID, "revision": float64(first.Revision)}, &detail)
	if detail.ID != first.ID || detail.Revision != first.Revision || detail.Content == "" || len(detail.Origins) == 0 || len(detail.References) == 0 {
		t.Fatalf("canonical detail=%+v", detail)
	}
	var next knowledge.View
	args := map[string]interface{}{"query": "evidence", "source": "claude", "limit": float64(1), "cursor": page.NextCursor}
	knowledgeResult(t, args, &next)
	if len(next.Items) != 1 || next.Items[0].ID == first.ID || next.Total != 2 {
		t.Fatalf("archive query-wide count/page=%+v", next)
	}
	// Find the versioned record irrespective of deterministic search order.
	id := first.ID
	if first.Revision == 1 {
		id = next.Items[0].ID
	}
	var history knowledge.HistoryPage
	knowledgeResult(t, map[string]interface{}{"mode": "history", "id": id, "limit": float64(1)}, &history)
	if len(history.Items) != 1 || history.Total != 2 || history.NextCursor == "" {
		t.Fatalf("version history=%+v", history)
	}
	var older knowledge.HistoryPage
	knowledgeResult(t, map[string]interface{}{"mode": "history", "id": id, "limit": float64(1), "cursor": history.NextCursor}, &older)
	if len(older.Items) != 1 || older.Items[0].Revision == history.Items[0].Revision || older.Total != 2 {
		t.Fatalf("older retained version=%+v", older)
	}
	raw, _ := json.Marshal([]any{page, next, detail, history, older})
	if strings.Contains(string(raw), "never-persist-this-secret") {
		t.Fatal("private identity leaked through read projection")
	}
	args["query"] = "changed"
	result, _ := handleMaatKnowledge(args)
	if !result.IsError {
		t.Fatal("archive cursor accepted changed filter")
	}
	if _, err = a.ImportSource(context.Background(), "claude", []seshat.SourceRecord{{Source: "claude", InstanceID: "inert", NativeItemID: "third", Schema: "fixture-v1", Title: "Reference", Content: "third evidence"}}, nil); err != nil {
		t.Fatal(err)
	}
	args["query"] = "evidence"
	result, _ = handleMaatKnowledge(args)
	if !result.IsError {
		t.Fatal("stale archive cursor accepted mutation")
	}
}
func TestMaatKnowledgeMCPMalformedBeforeHomeRead(t *testing.T) {
	old := maatKnowledgeHome
	t.Cleanup(func() { maatKnowledgeHome = old })
	reads := 0
	maatKnowledgeHome = func() (string, error) { reads++; return "must-not-read", nil }
	for _, args := range []map[string]interface{}{{"mode": "refresh"}, {"mode": "detail", "id": "not canonical"}, {"limit": float64(101)}, {"history": "true"}, {"cursor": "e30"}, {"classification": "made-up"}} {
		result, err := handleMaatKnowledge(args)
		if err != nil || !result.IsError {
			t.Fatalf("malformed archive read accepted=%+v", args)
		}
	}
	if reads != 0 {
		t.Fatalf("malformed input resolved home %d times", reads)
	}
}
