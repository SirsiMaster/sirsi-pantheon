package dashboard

import (
	"encoding/json"
	"net/http"
	"strings"

	buildversion "github.com/SirsiMaster/sirsi-pantheon/internal/version"
)

// routerSurfaceSchema is intentionally separate from the internal dashboard
// JSON shape. It is the small, stable read contract shared by the standalone
// router surface and Nexus. The producer remains RouterFn, so there is one
// authority and one failure boundary.
const routerSurfaceSchema = "router-surface.v1"

func (s *Server) surfaceHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Sirsi-Router-Schema", routerSurfaceSchema)
	if origin := strings.TrimRight(strings.TrimSpace(r.Header.Get("Origin")), "/"); origin != "" {
		if _, ok := s.surfaceOrigins[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Set("Vary", "Origin")
		}
	}
}

func (s *Server) handleRouterSurface(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/router" && r.URL.Path != "/router/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self'; frame-ancestors 'self' http://127.0.0.1:5173 http://localhost:5173 https://sirsi.ai")
	s.serveEmbedded(w, "ui/index.html", "text/html; charset=utf-8")
}

func (s *Server) apiRouterSurfaceManifest(w http.ResponseWriter, r *http.Request) {
	s.surfaceHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.RouterFn == nil {
		http.Error(w, `{"error":"router producer not configured"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"schema":    routerSurfaceSchema,
		"build":     buildversion.Current("sirsi"),
		"authority": "ra",
		"producer":  "pantheon.horus",
		"surface":   "/router",
		"endpoints": map[string]string{"snapshot": "/api/router/v1/snapshot"},
		"integrations": map[string]string{
			"pantheon": "same Horus producer",
			"nexus":    "read-only contract consumer",
		},
	})
}

func (s *Server) apiRouterSurfaceSnapshot(w http.ResponseWriter, r *http.Request) {
	s.surfaceHeaders(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if s.cfg.RouterFn == nil {
		http.Error(w, `{"error":"router producer not configured"}`, http.StatusServiceUnavailable)
		return
	}
	snap, err := s.cfg.RouterFn()
	if err != nil {
		http.Error(w, `{"error":"router producer unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snap)
}
