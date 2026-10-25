package mcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
)

type maatHistoryProjection struct {
	casebook.View
	JournalIntegrity maat.JournalIntegrity  `json:"journal_integrity"`
	History          casebook.HistoryWindow `json:"history"`
}

func readMaatHistory(j maat.DecisionJournal, q casebook.HistoryQuery) (maatHistoryProjection, error) {
	parent := q.Context
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, 2*time.Second)
	defer cancel()
	q.Context = ctx
	if file, ok := j.(*maat.FileDecisionJournal); ok {
		result, err := casebook.SearchHistory(file, q)
		if result.View.Cases == nil {
			result.View.Cases = []casebook.Case{}
		}
		return maatHistoryProjection{result.View, result.Integrity, result.Window}, err
	}
	// Compatibility for existing alternate transports. Never label a limited
	// Recent backend as a complete or snapshot-continuable history query.
	if q.Cursor != "" {
		return maatHistoryProjection{}, errors.New("backend cannot continue history")
	}
	rows, err := j.Recent(1000)
	if err != nil {
		return maatHistoryProjection{}, err
	}
	filtered := []maat.Decision{}
	for _, row := range rows {
		if q.Scope == maat.HistoryScopeSystemOne && row.SystemOne == nil && row.SystemOneCalibration == nil {
			continue
		}
		if q.Evidence != "" && row.Evidence != q.Evidence {
			continue
		}
		filtered = append(filtered, row)
	}
	view := casebook.Search(filtered, q.Query)
	return maatHistoryProjection{View: view, History: casebook.HistoryWindow{Scope: q.Scope, Returned: len(view.Cases), PageSize: q.Limit, Coverage: "unavailable", CountScope: "page"}}, nil
}

func parseMaatHistoryQuery(args map[string]interface{}) (casebook.HistoryQuery, error) {
	if err := knownMaatArgs(args, "query kind status limit scope evidence cursor"); err != nil {
		return casebook.HistoryQuery{}, err
	}
	base, err := parseMaatCasebookQuery(args)
	if err != nil {
		return casebook.HistoryQuery{}, err
	}
	q := casebook.HistoryQuery{Query: base, Scope: maat.HistoryScopeAll}
	if q.Scope, err = maatString(args, "scope", 32, q.Scope); err != nil {
		return q, err
	}
	if q.Scope != maat.HistoryScopeAll && q.Scope != maat.HistoryScopeSystemOne {
		return q, errors.New("scope must be all or system-one")
	}
	if len(q.Text) > 512 || len(q.Kind) > 128 || strings.ContainsAny(q.Text+q.Kind, "\x00\r\n") {
		return q, errors.New("casebook query exceeds its bound")
	}
	if q.Evidence, err = maatString(args, "evidence", 1024, ""); err != nil {
		return q, err
	}
	if q.Cursor, err = maatString(args, "cursor", 64<<10, ""); err != nil {
		return q, err
	}
	if err = maatCursor(q.Cursor, true); err != nil {
		return q, err
	}
	return q, nil
}

type maatKnowledgeRequest struct {
	Mode, ID string
	Revision int
	Query    knowledge.SearchQuery
}

func parseMaatKnowledgeRead(args map[string]interface{}) (maatKnowledgeRequest, error) {
	r := maatKnowledgeRequest{Mode: "search", Query: knowledge.SearchQuery{Limit: 50}}
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
	if r.Query.Classification != "" && !strings.Contains("|conversation|search|note|document|unclassified|", "|"+r.Query.Classification+"|") {
		return r, errors.New("invalid classification")
	}
	if resolution != "" && !strings.Contains("|open|reviewed|accepted|rejected|", "|"+resolution+"|") {
		return r, errors.New("invalid resolution")
	}
	if r.Mode == "search" {
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

func readMaatKnowledge(home string, r maatKnowledgeRequest) (any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if r.Mode == "search" {
		return knowledge.LoadPageContext(ctx, home, r.Query)
	}
	archive, err := knowledge.Open(home)
	if err != nil {
		return nil, err
	}
	defer func() { _ = archive.Close() }()
	if r.Mode == "detail" {
		return archive.Detail(ctx, r.ID, r.Revision)
	}
	return archive.HistoryPage(ctx, r.ID, r.Query.Limit, r.Query.Cursor)
}

func knownMaatArgs(args map[string]interface{}, keys string) error {
	for k := range args {
		if !strings.Contains(" "+keys+" ", " "+k+" ") {
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
		if !integer("v", 3, 3) || !integer("offset", 0, 1<<62) || !integer("limit", 1, 100) || text("snapshot", 4096) == "" || text("query", 128) == "" {
			return errors.New("invalid history continuation envelope")
		}
		switch phase {
		case "scan", "links", "candidate-links", "candidate-finalize", "materialize":
		default:
			return errors.New("invalid history continuation phase")
		}
	} else if !integer("g", 0, 1<<62) || !integer("o", 0, 1<<62) || text("q", 128) == "" {
		return errors.New("invalid archive continuation envelope")
	}
	return nil
}
