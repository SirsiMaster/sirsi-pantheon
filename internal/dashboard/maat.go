package dashboard

import (
	"net/http"
	"strings"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
	"github.com/SirsiMaster/sirsi-pantheon/internal/maat/casebook"
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
	if s.cfg.MaatCasebookFn == nil {
		writeError(w, "Ma'at casebook projection not available", http.StatusServiceUnavailable)
		return
	}
	status := casebook.Status(strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status"))))
	if status != "" && status != casebook.StatusOpen && status != casebook.StatusResolved {
		writeError(w, "invalid Ma'at casebook status", http.StatusBadRequest)
		return
	}
	view, err := s.cfg.MaatCasebookFn(casebook.Query{
		Text: r.URL.Query().Get("q"), Kind: r.URL.Query().Get("kind"), Status: status,
		Limit: parseIntParam(r, "limit", 50),
	})
	if err != nil {
		writeError(w, "Ma'at casebook projection failed", http.StatusInternalServerError)
		return
	}
	if view.Cases == nil {
		view.Cases = []casebook.Case{}
	}
	writeJSON(w, view)
}
