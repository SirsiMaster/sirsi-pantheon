package dashboard

import (
	"net/http"
	"strings"
)

// The tailnet entrance: `tailscale serve` terminates HTTPS on the Mac's
// MagicDNS name and forwards to a loopback port that serves only this handler,
// never the keyboard port. Tailscale stamps the requesting person on every
// forwarded request (and drops any client-supplied copy of these headers);
// a tagged node, a peer without a person behind it, or a person outside the
// allowed domains gets nothing.
const (
	tailnetUserHeader = "Tailscale-User-Login"
	// DefaultTailnetDomain is the only sign-in domain a Mac console admits by default.
	DefaultTailnetDomain = "sirsi.ai"
)

func tailnetOnly(next http.Handler, domains []string) http.Handler {
	if len(domains) == 0 {
		domains = []string{DefaultTailnetDomain}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		login := strings.ToLower(strings.TrimSpace(r.Header.Get(tailnetUserHeader)))
		if !loginInDomains(login, domains) {
			http.Error(w, "this console admits signed-in sirsi.ai tailnet users only", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/healthz" {
			// The board's fallback page checks consoles from another origin; it may
			// read this one answer, which says nothing but "up".
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !sameOrigin(r) {
			// A page on any other site could otherwise drive this console through
			// the viewer's own browser and tailnet identity.
			http.Error(w, "controls must come from this console's own page", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func loginInDomains(login string, domains []string) bool {
	_, domain, ok := strings.Cut(login, "@")
	if !ok || domain == "" {
		return false
	}
	for _, d := range domains {
		if strings.EqualFold(domain, strings.TrimSpace(d)) {
			return true
		}
	}
	return false
}

// sameOrigin requires the browser's own statement that the request came from
// this console's page: Sec-Fetch-Site same-origin, or an Origin equal to the
// served https origin. A request carrying neither is refused.
func sameOrigin(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin"
	}
	return r.Header.Get("Origin") == "https://"+r.Host
}
