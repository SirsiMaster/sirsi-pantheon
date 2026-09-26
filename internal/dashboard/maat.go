package dashboard

import (
	"net/http"

	"github.com/SirsiMaster/sirsi-pantheon/internal/maat"
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
