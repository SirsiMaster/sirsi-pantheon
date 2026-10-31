package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
)

func TestMaatKnowledgeHTTPQueryGuidanceBeforeProducer(t *testing.T) {
	reads := 0
	server := New(Config{MaatKnowledgeReadFn: func(context.Context, MaatKnowledgeReadRequest) (any, error) {
		reads++
		return nil, errors.New("must not read")
	}})
	cases := []struct{ name, text string }{
		{"513_bytes", strings.Repeat("x", 513)},
		{"NUL", "left\x00right"},
		{"CR", "left\rright"},
		{"LF", "left\nright"},
		{"invalid_UTF8", string([]byte{0xff})},
	}
	for _, tc := range cases {
		for _, key := range []string{"query", "q"} {
			t.Run(tc.name+"/"+key, func(t *testing.T) {
				response := httptest.NewRecorder()
				server.apiMaatKnowledge(response, httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?"+url.Values{key: []string{tc.text}}.Encode(), nil))
				var body struct{ Code, Message string }
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if response.Code != http.StatusBadRequest || body.Code != "invalid_search_query" || !strings.Contains(body.Message, "literal words") || reads != 0 {
					t.Fatalf("invalid text lost guidance or reached producer: HTTP%d %s reads=%d", response.Code, response.Body.String(), reads)
				}
			})
		}
	}
	// HTTP query values are strings, not JSON-typed values. A duplicate/alias
	// shape or unknown key is still a schema error, never a text-content error.
	for _, query := range []string{"query=one&query=two", "query=one&q=two", "xquery=one", "query=one&quer=two"} {
		response := httptest.NewRecorder()
		server.apiMaatKnowledge(response, httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?"+query, nil))
		var body struct{ Code string }
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusBadRequest || body.Code != "invalid_query" || reads != 0 {
			t.Fatalf("schema error misclassified or read private producer: HTTP%d %s", response.Code, response.Body.String())
		}
	}
}

func TestMaatKnowledgeHTTPParserKeepsWrongQueryTypesDistinct(t *testing.T) {
	// Non-string values cannot be supplied through a URL. Exercise the typed
	// parser seam directly; do not claim these are actual HTTP wire types.
	for _, value := range []any{float64(513), true, nil, []any{"word"}, map[string]any{"text": "word"}} {
		_, err := parseMaatKnowledgeRead(map[string]interface{}{"query": value})
		if err == nil || errors.Is(err, knowledge.ErrInvalidSearchQuery) || !strings.Contains(err.Error(), "query must be a string") {
			t.Fatalf("wrong query type lost schema distinction: %v", err)
		}
	}
}

func TestMaatKnowledgeHTTPQueryGuidancePreservesAcceptedText(t *testing.T) {
	for _, text := range []string{"", "archive-drift", strings.Repeat("x", 512)} {
		request, err := parseMaatKnowledgeHTTP(httptest.NewRequest(http.MethodGet, "/api/maat/knowledge?query="+url.QueryEscape(text), nil))
		if err != nil || request.Query.Text != text {
			t.Fatalf("valid literal/boundary changed: %v", err)
		}
	}
}
