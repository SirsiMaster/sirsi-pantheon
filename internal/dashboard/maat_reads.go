package dashboard

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
)

func maatReadError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "state": "unavailable", "code": code})
}

// Strict bounded transport parsing occurs before any producer/store read.
func maatHTTPArgs(r *http.Request) (map[string]interface{}, error) {
	if len(r.URL.RawQuery) > 256<<10 {
		return nil, errors.New("query too large")
	}
	values, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return nil, err
	}
	args := map[string]interface{}{}
	for k, v := range values {
		if len(v) != 1 {
			return nil, errors.New("duplicate argument")
		}
		key := k
		if key == "q" {
			key = "query"
		}
		if _, exists := args[key]; exists {
			return nil, errors.New("duplicate query alias")
		}
		switch key {
		case "limit", "revision":
			n, err := strconv.Atoi(v[0])
			if err != nil {
				return nil, err
			}
			args[key] = float64(n)
		case "history":
			if v[0] != "true" && v[0] != "false" {
				return nil, errors.New("invalid boolean")
			}
			args[key] = v[0] == "true"
		default:
			args[key] = v[0]
		}
	}
	return args, nil
}

func parseMaatHistoryHTTP(r *http.Request) (casebook.HistoryQuery, error) {
	args, err := maatHTTPArgs(r)
	q := casebook.HistoryQuery{Query: casebook.Query{Limit: 50}, Scope: maat.HistoryScopeAll}
	if err != nil {
		return q, err
	}
	if err = knownMaatArgs(args, "query kind status limit scope evidence cursor"); err != nil {
		return q, err
	}
	for _, f := range []struct {
		k   string
		p   *string
		max int
	}{{"query", &q.Text, 512}, {"kind", &q.Kind, 128}, {"scope", &q.Scope, 32}, {"evidence", &q.Evidence, 1024}, {"cursor", &q.Cursor, 64 << 10}} {
		*f.p, err = maatString(args, f.k, f.max, *f.p)
		if err != nil {
			return q, err
		}
	}
	status, err := maatString(args, "status", 32, "")
	if err != nil {
		return q, err
	}
	q.Status = casebook.Status(strings.ToLower(strings.TrimSpace(status)))
	if q.Status != "" && q.Status != casebook.StatusOpen && q.Status != casebook.StatusResolved {
		return q, errors.New("invalid status")
	}
	if q.Scope != maat.HistoryScopeAll && q.Scope != maat.HistoryScopeSystemOne {
		return q, errors.New("invalid scope")
	}
	if q.Limit, err = maatInt(args, "limit", 1, 100, 50); err != nil {
		return q, err
	}
	if err = maatCursor(q.Cursor, true); err != nil {
		return q, err
	}
	return q, nil
}
func parseMaatKnowledgeHTTP(r *http.Request) (MaatKnowledgeReadRequest, error) {
	args, err := maatHTTPArgs(r)
	if err != nil {
		return MaatKnowledgeReadRequest{}, err
	}
	return parseMaatKnowledgeRead(args)
}
func (r MaatKnowledgeReadRequest) legacyCompatible() bool {
	return r.Mode == "search" && r.Query.Limit == 50 && r.Query.Cursor == "" && r.Query.Source == "" && r.Query.Classification == "" && r.Query.Resolution == "" && !r.Query.IncludeHistory
}

type MaatKnowledgeReadRequest struct {
	Mode, ID string
	Revision int
	Query    knowledge.SearchQuery
}

func parseMaatKnowledgeRead(args map[string]interface{}) (MaatKnowledgeReadRequest, error) {
	r := MaatKnowledgeReadRequest{Mode: "search", Query: knowledge.SearchQuery{Limit: 50}}
	if err := knownMaatArgs(args, "mode query id revision limit cursor source classification resolution history"); err != nil {
		return r, err
	}
	var err error
	for _, f := range []struct {
		k   string
		p   *string
		max int
	}{{"mode", &r.Mode, 16}, {"id", &r.ID, 128}, {"query", &r.Query.Text, 512}, {"source", &r.Query.Source, 80}, {"classification", &r.Query.Classification, 48}, {"cursor", &r.Query.Cursor, 64 << 10}} {
		*f.p, err = maatString(args, f.k, f.max, *f.p)
		if err != nil {
			return r, err
		}
	}
	resolution, err := maatString(args, "resolution", 32, "")
	if err != nil {
		return r, err
	}
	r.Query.Resolution = knowledge.ResolutionState(resolution)
	if r.Query.Limit, err = maatInt(args, "limit", 1, 100, 50); err != nil {
		return r, err
	}
	if r.Revision, err = maatInt(args, "revision", 0, 1000000000, 0); err != nil {
		return r, err
	}
	if v, ok := args["history"]; ok {
		b, valid := v.(bool)
		if !valid {
			return r, errors.New("history must be boolean")
		}
		r.Query.IncludeHistory = b
	}
	if r.Mode != "search" && r.Mode != "detail" && r.Mode != "history" {
		return r, errors.New("mode must be search, detail or history")
	}
	switch r.Query.Classification {
	case "", "conversation", "search", "note", "document", "unclassified":
	default:
		return r, errors.New("invalid classification")
	}
	switch resolution {
	case "", "open", "reviewed", "accepted", "rejected":
	default:
		return r, errors.New("invalid resolution")
	}
	if r.Mode == "search" {
		if err := knowledge.ValidateSearchText(r.Query.Text); err != nil {
			return r, err
		}
		if r.ID != "" || r.Revision != 0 {
			return r, errors.New("search cannot select a detail identity")
		}
	} else {
		if len(r.ID) != 64 || strings.Trim(r.ID, "0123456789abcdef") != "" {
			return r, errors.New("detail/history requires a canonical record ID")
		}
		if r.Query.Text != "" || r.Query.Source != "" || r.Query.Classification != "" || resolution != "" || r.Query.IncludeHistory {
			return r, errors.New("detail/history cannot discard search filters")
		}
		if r.Mode == "detail" && r.Query.Cursor != "" || r.Mode == "history" && r.Revision != 0 {
			return r, errors.New("incompatible detail/history options")
		}
	}
	if err = maatCursor(r.Query.Cursor, false); err != nil {
		return r, err
	}
	return r, nil
}

func knownMaatArgs(args map[string]interface{}, keys string) error {
	allowed := make(map[string]struct{})
	for _, key := range strings.Fields(keys) {
		allowed[key] = struct{}{}
	}
	for key := range args {
		if _, ok := allowed[key]; !ok {
			return errors.New("unknown Ma'at read argument")
		}
	}
	return nil
}
func maatString(args map[string]interface{}, key string, max int, fallback string) (string, error) {
	v, ok := args[key]
	if !ok {
		return fallback, nil
	}
	s, ok := v.(string)
	if !ok || len(s) > max || strings.ContainsAny(s, "\x00\r\n") {
		return "", errors.New("invalid bounded string: " + key)
	}
	return s, nil
}
func maatInt(args map[string]interface{}, key string, min, max, def int) (int, error) {
	v, ok := args[key]
	if !ok {
		return def, nil
	}
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) || n < float64(min) || n > float64(max) || math.Trunc(n) != n {
		return 0, errors.New("invalid bounded integer: " + key)
	}
	return int(n), nil
}

// Only transport syntax is checked here; snapshot/query/file-position
// authority remains in the canonical reader, never this adapter.
func maatCursor(value string, history bool) error {
	if value == "" {
		return nil
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	var fields map[string]json.RawMessage
	if err != nil || json.Unmarshal(b, &fields) != nil || fields == nil {
		return errors.New("invalid continuation encoding")
	}
	integer := func(k string, min, max int64) bool {
		var n int64
		raw, ok := fields[k]
		return ok && string(raw) != "null" && json.Unmarshal(raw, &n) == nil && n >= min && n <= max
	}
	text := func(k string, max int) string {
		var v string
		if json.Unmarshal(fields[k], &v) != nil || len(v) > max || strings.ContainsAny(v, "\x00\r\n") {
			return ""
		}
		return v
	}
	if history {
		phase := text("phase", 32)
		if !integer("v", 4, 4) || !integer("offset", 0, 1<<62) || !integer("limit", 1, 100) || text("snapshot", 4096) == "" || text("query", 128) == "" {
			return errors.New("invalid history continuation envelope")
		}
		switch phase {
		case "scan", "links", "candidate-links", "candidate-finalize", "materialize", "batch-links", "batch-finalize":
		default:
			return errors.New("invalid history continuation phase")
		}
	} else if !integer("g", 0, 1<<62) || !integer("o", 0, 1<<62) || text("q", 128) == "" {
		return errors.New("invalid archive continuation envelope")
	}
	return nil
}
