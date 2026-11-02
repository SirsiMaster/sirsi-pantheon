package dashboard

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// The tailnet entrance: `tailscale serve` terminates HTTPS on the Mac's
// MagicDNS name and forwards to a listener that serves only this handler,
// never the keyboard port. Tailscale stamps the requesting person on every
// forwarded request (and drops any client-supplied copy of these headers);
// a tagged node, a peer without a person behind it, or a person outside the
// allowed domains gets nothing.
//
// Trust boundary: this handler believes the identity header, so whatever can
// reach its listener directly can claim any sign-in. Two targets:
//   - a Unix socket (TailnetSocket, recommended): 0600 inside a 0700 directory,
//     so only this user's processes and tailscaled can connect;
//   - a loopback TCP port (TailnetPort): any process on this Mac can connect.
//     Same-host processes are treated as inside the boundary — they can already
//     run the sirsi CLI as this user — and nothing off the Mac can reach it.
const (
	tailnetUserHeader = "Tailscale-User-Login"
	// DefaultTailnetDomain is the only sign-in domain a Mac console admits by default.
	DefaultTailnetDomain = "sirsi.ai"
)

func tailnetOnly(next http.Handler, domains []string, loopbackOnly bool) http.Handler {
	if len(domains) == 0 {
		domains = []string{DefaultTailnetDomain}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if loopbackOnly && !fromLoopback(r.RemoteAddr) {
			// Defense in depth: the TCP entrance is bound to 127.0.0.1, so only the
			// local proxy should ever be the peer.
			http.Error(w, "this entrance answers only the local tailscale proxy", http.StatusForbidden)
			return
		}
		// Tailscale sets exactly one identity; two copies or a blank one is not its stamp.
		logins := r.Header.Values(tailnetUserHeader)
		login := ""
		if len(logins) == 1 {
			login = strings.ToLower(strings.TrimSpace(logins[0]))
		}
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
		if r.URL.Path == "/whoami" {
			// Who this entrance admitted — lets the host lane prove a forged header
			// never wins over Tailscale's own stamp.
			w.Header().Set("Cache-Control", "no-store")
			_, _ = fmt.Fprintln(w, login)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func fromLoopback(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	ip := net.ParseIP(host)
	return err == nil && ip != nil && ip.IsLoopback()
}

// tailnetConfigErr refuses a tailnet entrance that would share a door with an
// unguarded one: if `tailscale serve` targets a port that also serves the plain
// handler, sign-in checks are skipped for whoever wins the bind.
func tailnetConfigErr(cfg Config) error {
	if cfg.TailnetPort != 0 && cfg.TailnetSocket != "" {
		return errors.New("tailnet port and tailnet socket are exclusive; pick one")
	}
	if cfg.TailnetPort != 0 && (cfg.TailnetPort == cfg.Port || slices.Contains(cfg.AltPorts, cfg.TailnetPort)) {
		return fmt.Errorf("tailnet port %d is also an unguarded console port; `tailscale serve` would reach it without sign-in checks", cfg.TailnetPort)
	}
	return nil
}

// listenTailnet binds the protected entrance before the console reports
// running, so a failed bind stops startup instead of leaving the proxy target
// unserved (or served by something else).
func listenTailnet(cfg Config) (net.Listener, error) {
	if cfg.TailnetSocket == "" {
		return net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", cfg.TailnetPort))
	}
	dir := filepath.Dir(cfg.TailnetSocket)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if fi, err := os.Stat(dir); err != nil {
		return nil, err
	} else if fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s is %v; the socket directory must be private (0700)", dir, fi.Mode().Perm())
	}
	if fi, err := os.Lstat(cfg.TailnetSocket); err == nil {
		if fi.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("%s exists and is not a socket", cfg.TailnetSocket)
		}
		// A previous run's socket; the dashboard host lock means no live owner.
		if err := os.Remove(cfg.TailnetSocket); err != nil {
			return nil, err
		}
	}
	l, err := net.Listen("unix", cfg.TailnetSocket)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(cfg.TailnetSocket, 0o600); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
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
