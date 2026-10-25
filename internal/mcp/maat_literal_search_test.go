package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
	"github.com/SirsiMaster/sirsi-pantheon/internal/seshat"
)

func TestMaatKnowledgeMCPLiteralSearch(t *testing.T) {
	home := filepath.Join(t.TempDir(), "private-home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	archive, err := knowledge.Open(home)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := archive.Close(); err != nil {
			t.Error(err)
		}
	})
	raw, err := os.ReadFile("../maat/knowledge/testdata/literal-search.json")
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
	oldHome := maatKnowledgeHome
	t.Cleanup(func() { maatKnowledgeHome = oldHome })
	maatKnowledgeHome = func() (string, error) { return home, nil }

	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			var page knowledge.View
			knowledgeResult(t, map[string]interface{}{"query": tc.Query, "source": "claude"}, &page)
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
			reads := 0
			maatKnowledgeHome = func() (string, error) { reads++; return home, nil }
			result, err := handleMaatKnowledge(map[string]interface{}{"query": query})
			if err != nil || !result.IsError || !strings.Contains(result.Content[0].Text, "invalid_search_query") || reads != 0 {
				t.Fatalf("malformed literal query reached HOME or lost guidance: %+v %v reads=%d", result, err, reads)
			}
		})
	}
}
