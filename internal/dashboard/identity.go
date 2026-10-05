package dashboard

import "net/http"

// apiIdentity is deliberately read-only. It lets an operator identify the
// process serving the dashboard without granting process, engine, or
// control-plane authority.
func (s *Server) apiIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, IdentityResponse{Schema: IdentitySchema, Info: s.cfg.BuildIdentity})
}
