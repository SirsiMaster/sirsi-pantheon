package dashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
)

func TestMaatHTTPExactMembershipRefusesBeforePrivateProducer(t *testing.T) {
	reads := 0
	server := New(Config{
		MaatHistoryFn: func(context.Context, casebook.HistoryQuery) (casebook.HistorySearchResult, error) {
			reads++
			return casebook.HistorySearchResult{}, nil
		},
		MaatKnowledgeReadFn: func(context.Context, MaatKnowledgeReadRequest) (any, error) {
			reads++
			return map[string]interface{}{"items": []interface{}{}, "total": 0}, nil
		},
	})
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
			reads = 0
			path := "/api/maat/knowledge"
			if tc.casebook {
				path = "/api/maat/casebook"
			}
			query := url.Values{tc.key: []string{tc.value}}
			request := httptest.NewRequest(http.MethodGet, path+"?"+query.Encode(), nil)
			recorder := httptest.NewRecorder()
			if tc.casebook {
				server.apiMaatCasebook(recorder, request)
			} else {
				server.apiMaatKnowledge(recorder, request)
			}
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("malformed transport status=%d body=%s", recorder.Code, recorder.Body.String())
			}
			if reads != 0 {
				t.Fatalf("malformed transport reached %d private producers", reads)
			}
		})
	}
}

func TestMaatHTTPExactMembershipPreservesLiteralsAliasesAndDefaults(t *testing.T) {
	for _, classification := range []string{"", "conversation", "search", "note", "document", "unclassified"} {
		req := httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?"+url.Values{"classification": []string{classification}}.Encode(), nil)
		r, err := parseMaatKnowledgeHTTP(req)
		if err != nil || r.Query.Classification != classification || r.Query.Limit != 50 || r.Mode != "search" {
			t.Fatalf("supported classification/defaults changed: %+v %v", r, err)
		}
	}
	for _, resolution := range []string{"", "open", "reviewed", "accepted", "rejected"} {
		req := httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?"+url.Values{"resolution": []string{resolution}}.Encode(), nil)
		r, err := parseMaatKnowledgeHTTP(req)
		if err != nil || string(r.Query.Resolution) != resolution {
			t.Fatalf("supported resolution changed: %+v %v", r, err)
		}
	}
	for _, key := range []string{"q", "query"} {
		req := httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?"+url.Values{key: []string{"retained"}}.Encode(), nil)
		r, err := parseMaatKnowledgeHTTP(req)
		if err != nil || r.Query.Text != "retained" {
			t.Fatalf("legacy query alias %s changed: %+v %v", key, r, err)
		}
		history, err := parseMaatHistoryHTTP(req)
		if err != nil || history.Text != "retained" || history.Limit != 50 || history.Scope != "all" {
			t.Fatalf("casebook query alias/defaults changed: %+v %v", history, err)
		}
	}
}
