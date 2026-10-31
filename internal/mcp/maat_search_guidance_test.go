package mcp

import (
	"errors"
	"strings"
	"testing"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
)

func TestMaatKnowledgeMCPQueryGuidanceBeforeHome(t *testing.T) {
	oldHome := maatKnowledgeHome
	t.Cleanup(func() { maatKnowledgeHome = oldHome })
	reads := 0
	maatKnowledgeHome = func() (string, error) { reads++; return "must-not-read", nil }
	cases := []struct {
		name      string
		value     any
		textError bool
	}{
		{"513_bytes", strings.Repeat("x", 513), true},
		{"NUL", "left\x00right", true},
		{"CR", "left\rright", true},
		{"LF", "left\nright", true},
		{"invalid_UTF8", string([]byte{0xff}), true},
		{"number", float64(513), false},
		{"boolean", true, false},
		{"null", nil, false},
		{"array", []any{"word"}, false},
		{"object", map[string]any{"text": "word"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			args := map[string]interface{}{"query": tc.value}
			_, parseErr := parseMaatKnowledgeRead(args)
			if parseErr == nil || errors.Is(parseErr, knowledge.ErrInvalidSearchQuery) != tc.textError {
				t.Fatalf("wrong query error category: %v", parseErr)
			}
			result, err := handleMaatKnowledge(args)
			if err != nil || !result.IsError || len(result.Content) != 1 || reads != 0 {
				t.Fatalf("invalid query reached HOME: %+v %v reads=%d", result, err, reads)
			}
			message := result.Content[0].Text
			if tc.textError {
				if !strings.Contains(message, "invalid_search_query") || !strings.Contains(message, "literal words") {
					t.Fatalf("missing query guidance: %s", message)
				}
			} else if !strings.Contains(message, "query must be a string") || strings.Contains(message, "invalid_search_query") {
				t.Fatalf("wrong-type error disguised as text error: %s", message)
			}
		})
	}
	// Unknown names remain a closed-schema error even alongside invalid text.
	_, err := parseMaatKnowledgeRead(map[string]interface{}{"query": strings.Repeat("x", 513), "xquery": "ignored"})
	if err == nil || errors.Is(err, knowledge.ErrInvalidSearchQuery) || !strings.Contains(err.Error(), "unknown Ma'at read argument") {
		t.Fatalf("unknown key lost precedence: %v", err)
	}
	if reads != 0 {
		t.Fatalf("private HOME reads=%d", reads)
	}
}

func TestMaatKnowledgeMCPQueryGuidancePreservesAcceptedText(t *testing.T) {
	for _, text := range []string{"", "archive-drift", strings.Repeat("x", 512)} {
		request, err := parseMaatKnowledgeRead(map[string]interface{}{"query": text})
		if err != nil || request.Query.Text != text {
			t.Fatalf("valid literal/boundary changed: %v", err)
		}
	}
}
