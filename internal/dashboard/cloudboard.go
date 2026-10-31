package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strings"
)

// cloudBoardPaths are served by the hosted board itself: the router surface,
// its read-only producers and the installable-app shell. Everything else is a
// workstation path (controls, host stats, classic pages) and is forwarded to
// the Mac console the viewer selected — the Mac runs it through its own
// handlers, action registry and confirm guard, exactly as at the keyboard.
var cloudBoardPaths = []string{"/", "/router", "/router/", "/api/router", "/api/fabric", "/healthz",
	"/api/macs", "/sw.js", "/offline.html", "/manifest.webmanifest"}

var cloudBoardPrefixes = []string{"/assets/", "/api/router/v1/"}

// macCookie names the Mac console a viewer is driving. It is a selection, not a
// credential: who may reach the board is decided by IAP in front of it.
const macCookie = "sirsi_mac"

// actorHeader carries the signed-in viewer to the Mac console for its audit
// trail. Any client-supplied value is discarded; only IAP's header is used.
const actorHeader = "X-Sirsi-Actor"

const iapEmailHeader = "X-Goog-Authenticated-User-Email"

// macConsoles is the current name → console URL map: discovered live when
// MacConsolesFn is set, otherwise the static list.
func (s *Server) macConsoles() map[string]string {
	if s.cfg.MacConsolesFn != nil {
		return s.cfg.MacConsolesFn()
	}
	return s.cfg.MacConsoles
}

func (s *Server) macProxy(name string) *httputil.ReverseProxy {
	target, err := url.Parse(s.macConsoles()[name])
	if name == "" || err != nil || target.Scheme == "" || target.Host == "" {
		return nil
	}
	return &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(target)
			// The Mac console has no use for the viewer's board cookies (the IAP session included).
			pr.Out.Header.Del("Cookie")
		},
		Transport: s.cfg.MacTransport,
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeError(w, "Mac console "+name+" unreachable: "+err.Error(), http.StatusBadGateway)
		},
	}
}

func (s *Server) cloudBoardOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Header.Del(actorHeader)
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte("ok\n"))
			return
		case r.URL.Path == "/api/macs":
			s.apiMacs(w, r)
			return
		case strings.HasPrefix(r.URL.Path, "/mac/"):
			s.selectMac(w, r)
			return
		case cloudBoardAllows(r.URL.Path):
			if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		name := selectedMac(r)
		p := s.macProxy(name)
		if p == nil {
			writeError(w, "choose a Mac first: this control runs on a Mac, not in the cloud", http.StatusConflict)
			return
		}
		if actor := strings.TrimPrefix(r.Header.Get(iapEmailHeader), "accounts.google.com:"); actor != "" {
			r.Header.Set(actorHeader, actor)
		}
		p.ServeHTTP(w, r)
	})
}

func selectedMac(r *http.Request) string {
	c, err := r.Cookie(macCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

// selectMac handles GET /mac/<name>: remember the Mac this viewer drives and
// return to its Host view. An unknown name is refused, never stored.
func (s *Server) selectMac(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/mac/")
	if _, ok := s.macConsoles()[name]; !ok {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: macCookie, Value: name, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/#host", http.StatusSeeOther)
}

// apiMacs lists the Mac consoles this board can drive and which one this
// viewer has selected. Its presence is also how the page knows it is the
// hosted board (the local console has no /api/macs).
func (s *Server) apiMacs(w http.ResponseWriter, r *http.Request) {
	macs := s.macConsoles()
	names := make([]string, 0, len(macs))
	for n := range macs {
		names = append(names, n)
	}
	sort.Strings(names)
	consoles := make([]map[string]string, 0, len(names))
	for _, n := range names {
		consoles = append(consoles, map[string]string{"name": n, "url": macs[n]})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"macs": consoles, "selected": selectedMac(r)})
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
