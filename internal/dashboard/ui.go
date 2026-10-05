package dashboard

import (
	"embed"
	"fmt"
	"net/http"

	"github.com/SirsiMaster/sirsi-pantheon/internal/brand"
)

// The dashboard UI: static files (no framework, no build step) embedded in the
// binary. Colors are not in these files: tokens.css is generated from
// internal/brand (ADR-038) so the dashboard cannot drift from the CLI, menubar
// and Swift app.
//
//go:embed ui/index.html ui/app.css ui/app.js
var uiFS embed.FS

// handleHome serves the dashboard at exactly "/". The previous terminal-style page
// remains at /classic for the scan, ghosts, guard and vault tools.
func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := uiFS.ReadFile("ui/index.html")
	if err != nil {
		http.Error(w, "ui unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}

func (s *Server) handleUIAsset(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/assets/tokens.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		fmt.Fprintf(w, ":root{color-scheme:light dark;\n%s}\n@media (prefers-color-scheme: dark){:root{\n%s}}\n", brand.CSSVars(brand.Light), brand.CSSVars(brand.Dark))
	case "/assets/app.css":
		s.serveEmbedded(w, "ui/app.css", "text/css; charset=utf-8")
	case "/assets/app.js":
		s.serveEmbedded(w, "ui/app.js", "application/javascript; charset=utf-8")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) serveEmbedded(w http.ResponseWriter, name, ctype string) {
	b, err := uiFS.ReadFile(name)
	if err != nil {
		http.Error(w, "asset unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b)
}
