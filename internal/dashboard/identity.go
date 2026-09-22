package dashboard

import "net/http"

// apiIdentity is deliberately read-only. It lets an operator prove that a
// running dashboard is the intended build before presenting it, without
// granting any process, engine, or control-plane authority.
func (s *Server) apiIdentity(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, IdentityResponse{Schema: IdentitySchema, Info: s.cfg.BuildIdentity})
}
