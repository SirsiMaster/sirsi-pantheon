package dashboard

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// cloudBoardPaths is everything a hosted board serves: the router surface page,
// its assets and its read-only producers. Workstation endpoints (run, clean,
// slay, vault, guard, ghosts, ask …) act on the host they run on; in the cloud
// that host is the board's own container, so they are not reachable at all.
// Mac controls are never relayed: the board only links to each Mac's console.
var cloudBoardPaths = []string{"/", "/router", "/router/", "/api/router", "/api/fabric", "/healthz",
	"/api/macs", "/sw.js", "/offline.html", "/manifest.webmanifest", "/consoleprobe.js"}

var cloudBoardPrefixes = []string{"/assets/", "/api/router/v1/"}

func cloudBoardOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			_, _ = w.Write([]byte("ok\n"))
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			w.WriteHeader(http.StatusMethodNotAllowed)
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

// apiMacs lists the Mac consoles the board links to. Its presence is also how
// the page knows it is the hosted board: a Mac's own console has no list.
func (s *Server) apiMacs(w http.ResponseWriter, r *http.Request) {
	if len(s.cfg.Macs) == 0 {
		http.NotFound(w, r)
		return
	}
	names := make([]string, 0, len(s.cfg.Macs))
	for n := range s.cfg.Macs {
		names = append(names, n)
	}
	sort.Strings(names)
	consoles := make([]map[string]string, 0, len(names))
	for _, n := range names {
		consoles = append(consoles, map[string]string{"name": n, "url": s.cfg.Macs[n]})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"macs": consoles})
}
