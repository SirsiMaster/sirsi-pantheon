package mcp

import (
	"errors"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
)

func TestMaatTransportExactMembershipRefusesBeforePrivateReaders(t *testing.T) {
	oldHome, oldJournal := maatKnowledgeHome, openMaatCasebookJournal
	t.Cleanup(func() { maatKnowledgeHome = oldHome; openMaatCasebookJournal = oldJournal })
	homeReads, journalReads := 0, 0
	maatKnowledgeHome = func() (string, error) { homeReads++; return "", errors.New("private home must not be requested") }
	openMaatCasebookJournal = func() (maat.DecisionJournal, error) {
		journalReads++
		return nil, errors.New("private journal must not be requested")
	}
	cases := []struct {
		name, key, value string
		casebook         bool
	}{
		{"casebook-composite-key", "query kind", "ignored", true},
		{"casebook-whole-key-list", "query kind status limit scope evidence cursor", "ignored", true},
		{"casebook-prefix-key", "xquery", "ignored", true},
		{"casebook-suffix-key", "queryx", "ignored", true},
		{"casebook-substring-key", "quer", "ignored", true},
		{"knowledge-composite-key", "mode query", "ignored", false},
		{"knowledge-composite-filter-keys", "classification resolution", "ignored", false},
		{"knowledge-composite-page-keys", "limit cursor source", "ignored", false},
		{"knowledge-prefix-key", "xclassification", "ignored", false},
		{"knowledge-suffix-key", "classificationx", "ignored", false},
		{"knowledge-substring-key", "classificatio", "ignored", false},
		{"class-composite", "classification", "conversation|search", false},
		{"class-adjacent-subset", "classification", "search|note", false},
		{"class-whole-list", "classification", "conversation|search|note|document|unclassified", false},
		{"class-prefix", "classification", "xconversation", false},
		{"class-suffix", "classification", "conversationx", false},
		{"class-substring", "classification", "versatio", false},
		{"resolution-composite", "resolution", "open|reviewed", false},
		{"resolution-adjacent-subset", "resolution", "reviewed|accepted", false},
		{"resolution-whole-list", "resolution", "open|reviewed|accepted|rejected", false},
		{"resolution-prefix", "resolution", "xopen", false},
		{"resolution-suffix", "resolution", "openx", false},
		{"resolution-substring", "resolution", "pen", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			homeReads, journalReads = 0, 0
			args := map[string]interface{}{tc.key: tc.value}
			var result *ToolResult
			var err error
			if tc.casebook {
				result, err = handleMaatCasebook(args)
			} else {
				result, err = handleMaatKnowledge(args)
			}
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("malformed transport accepted: %+v %v", result, err)
			}
			if homeReads != 0 || journalReads != 0 {
				t.Fatalf("malformed transport reached private readers: home=%d journal=%d", homeReads, journalReads)
			}
		})
	}
}

func TestMaatTransportExactMembershipPreservesLiteralsAndDefaults(t *testing.T) {
	for _, classification := range []string{"", "conversation", "search", "note", "document", "unclassified"} {
		r, err := parseMaatKnowledgeRead(map[string]interface{}{"classification": classification})
		if err != nil || r.Query.Classification != classification || r.Mode != "search" || r.Query.Limit != 50 {
			t.Fatalf("supported classification/defaults changed: %+v %v", r, err)
		}
	}
	for _, resolution := range []string{"", "open", "reviewed", "accepted", "rejected"} {
		r, err := parseMaatKnowledgeRead(map[string]interface{}{"resolution": resolution})
		if err != nil || string(r.Query.Resolution) != resolution {
			t.Fatalf("supported resolution changed: %+v %v", r, err)
		}
	}
	for key, value := range map[string]interface{}{"mode": "search", "query": "", "id": "", "revision": float64(0), "limit": float64(50), "cursor": "", "source": "", "classification": "", "resolution": "", "history": false} {
		if _, err := parseMaatKnowledgeRead(map[string]interface{}{key: value}); err != nil {
			t.Fatalf("supported key %q refused: %v", key, err)
		}
	}
	for key, value := range map[string]interface{}{"query": "", "kind": "", "status": "", "limit": float64(50), "scope": "all", "evidence": "", "cursor": ""} {
		if _, err := parseMaatHistoryQuery(map[string]interface{}{key: value}); err != nil {
			t.Fatalf("supported casebook key %q refused: %v", key, err)
		}
	}
}
