package dashboard

import (
	"encoding/json"
	"net/http"
)

// apiVitals is an explicit read of the ranked CLI producer, separate from the
// periodic capacity feed. It never repairs, signals or changes permissions.
func (s *Server) apiVitals(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.VitalsFn == nil {
		writeError(w, "memory diagnostics unavailable", http.StatusServiceUnavailable)
		return
	}
	data, err := s.cfg.VitalsFn()
	if err != nil || !json.Valid(data) {
		writeError(w, "memory diagnostics could not be read; try Inspect memory again", http.StatusInternalServerError)
		return
	}
	writeJSON(w, json.RawMessage(data))
}
