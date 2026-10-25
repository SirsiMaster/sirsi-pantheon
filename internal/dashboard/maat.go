package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/knowledge"
)

// apiMaatDecisions exposes recorded Ma'at assessments for the dashboard's
// drill-down view. It has no write verb and never turns a factual record into
// an authorization.
func (s *Server) apiMaatDecisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.MaatDecisionsFn == nil {
		writeError(w, "Ma'at decision projection not available", http.StatusServiceUnavailable)
		return
	}
	decisions, err := s.cfg.MaatDecisionsFn(parseIntParam(r, "limit", 50))
	if err != nil {
		writeError(w, "Ma'at decision projection failed", http.StatusInternalServerError)
		return
	}
	if decisions == nil {
		decisions = []maat.Decision{}
	}
	writeJSON(w, decisions)
}

// apiMaatCasebook exposes Ma'at's local System One projection. It is
// intentionally a GET-only read model over the decision journal: it does not
// calculate policy, make a reservation, or create authorization.
func (s *Server) apiMaatCasebook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query, parseErr := parseMaatHistoryHTTP(r)
	if parseErr != nil {
		maatReadError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	if s.cfg.MaatCasebookFn == nil && s.cfg.MaatHistoryFn == nil {
		writeError(w, "Ma'at casebook projection not available", http.StatusServiceUnavailable)
		return
	}
	var result casebook.HistorySearchResult
	var err error
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if s.cfg.MaatHistoryFn != nil {
		result, err = s.cfg.MaatHistoryFn(ctx, query)
	} else {
		if query.Cursor != "" || query.Evidence != "" || query.Scope != maat.HistoryScopeAll {
			maatReadError(w, http.StatusServiceUnavailable, "history_unavailable")
			return
		}
		result.View, err = s.cfg.MaatCasebookFn(query.Query)
		result.Window = casebook.HistoryWindow{Scope: query.Scope, Returned: len(result.View.Cases), PageSize: query.Limit, Coverage: "unavailable", CountScope: "page"}
	}
	if err != nil {
		if errors.Is(err, maat.ErrHistorySnapshotChanged) {
			maatReadError(w, http.StatusConflict, "snapshot_changed")
		} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			maatReadError(w, http.StatusRequestTimeout, "read_cancelled")
		} else {
			maatReadError(w, http.StatusServiceUnavailable, "history_unavailable")
		}
		return
	}
	view := result.View
	if result.Window.Coverage == maat.HistoryCoveragePartial {
		view.Cases = []casebook.Case{}
		view.Summary = casebook.Summary{}
	}
	if view.Cases == nil {
		view.Cases = []casebook.Case{}
	}
	writeJSON(w, struct {
		casebook.View
		History   casebook.HistoryWindow `json:"history"`
		Integrity maat.JournalIntegrity  `json:"journal_integrity"`
	}{view, result.Window, result.Integrity})
}

// apiMaatKnowledge projects the same sensitivity-filtered Ma'at local
// knowledge view through the dashboard. It is a GET-only presentation route:
// source ingestion, raw-cache access, and export stay outside this surface.
func (s *Server) apiMaatKnowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	query, parseErr := parseMaatKnowledgeHTTP(r)
	if parseErr != nil {
		if errors.Is(parseErr, knowledge.ErrInvalidSearchQuery) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_search_query", "code": "invalid_search_query", "state": "unavailable", "message": knowledge.ErrInvalidSearchQuery.Error()})
			return
		}
		maatReadError(w, http.StatusBadRequest, "invalid_query")
		return
	}
	if s.cfg.MaatKnowledgeFn == nil && s.cfg.MaatKnowledgeReadFn == nil {
		writeError(w, "Ma'at knowledge projection not available", http.StatusServiceUnavailable)
		return
	}
	if s.cfg.MaatKnowledgeReadFn != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		view, err := s.cfg.MaatKnowledgeReadFn(ctx, query)
		if err != nil {
			maatReadError(w, http.StatusServiceUnavailable, "knowledge_unavailable_or_stale")
			return
		}
		writeJSON(w, view)
		return
	}
	if !query.legacyCompatible() {
		maatReadError(w, http.StatusServiceUnavailable, "knowledge_paging_unavailable")
		return
	}
	view, err := s.cfg.MaatKnowledgeFn(query.Query.Text)
	if err != nil {
		writeError(w, "Ma'at knowledge projection failed", http.StatusInternalServerError)
		return
	}
	if view.Items == nil {
		view.Items = []knowledge.Item{}
	}
	writeJSON(w, view)
}
