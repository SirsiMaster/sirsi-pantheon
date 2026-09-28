package dashboard

import "net/http"

// apiApolloTelemetry exposes exactly one SNE-owned typed local session sample.
// It is read-only: the dashboard cannot start, stop, configure, or infer an
// Apollo/SNE session. A missing producer is unavailable, never an empty active
// telemetry record.
func (s *Server) apiApolloTelemetry(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.ApolloTelemetryFn == nil {
		writeError(w, "Apollo telemetry projection not available", http.StatusServiceUnavailable)
		return
	}
	read, err := s.cfg.ApolloTelemetryFn()
	if err != nil {
		writeError(w, "Apollo telemetry projection failed", http.StatusInternalServerError)
		return
	}
	writeJSON(w, read)
}
