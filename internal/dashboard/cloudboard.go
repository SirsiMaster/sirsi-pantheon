package dashboard

import (
	"net/http"
	"strings"
)

// cloudBoardPaths is everything a hosted board serves: the router surface page,
// its assets and its read-only producers. Workstation endpoints (run, clean,
// slay, vault, guard, ghosts, ask …) act on the host they run on; in the cloud
// that host is the board's own container, so they are not reachable at all.
var cloudBoardPaths = []string{"/", "/router", "/router/", "/api/router", "/api/fabric", "/healthz"}

var cloudBoardPrefixes = []string{"/assets/", "/api/router/v1/"}

func cloudBoardOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The ledger-edit API is the one write path; it checks identity and origin itself (boardact.go).
		edit := r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, boardActPrefix)
		if !edit && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		if cloudBoardAllows(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func cloudBoardAllows(path string) bool {
	for _, p := range cloudBoardPaths {
		if path == p {
			return true
		}
	}
	for _, p := range cloudBoardPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}
