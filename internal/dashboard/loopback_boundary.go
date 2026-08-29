package dashboard

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

const sneLocalSessionCookie = "sirsi_sne_local_session"

type sneLocalAccess struct {
	mu    sync.RWMutex
	token []byte
}

func newSNELocalAccess(token string) *sneLocalAccess {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return &sneLocalAccess{token: []byte(token)}
}

func (access *sneLocalAccess) matches(provided []byte) bool {
	access.mu.RLock()
	defer access.mu.RUnlock()
	return len(provided) == len(access.token) && subtle.ConstantTimeCompare(provided, access.token) == 1
}

func (access *sneLocalAccess) replace(token string) {
	access.mu.Lock()
	defer access.mu.Unlock()
	access.token = []byte(token)
}

func (access *sneLocalAccess) snapshot() string {
	if access == nil {
		return ""
	}
	access.mu.RLock()
	defer access.mu.RUnlock()
	return string(access.token)
}

// requireLoopbackHost rejects DNS-rebinding requests before they reach local
// SNE or recovery handlers. Binding the listener to 127.0.0.1 is not enough:
// a hostile hostname can resolve to loopback while retaining an attacker-
// controlled Host header.
func requireLoopbackHost(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !isLoopbackRequestHost(r.Host) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			writeError(w, "loopback Host required", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func (s *Server) requireSNECapability(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowNexusOrigin(w, r)
		origin := strings.TrimSpace(r.Header.Get("Origin"))
		if origin != "" && !sameOriginRequest(r) && w.Header().Get("Access-Control-Allow-Origin") == "" {
			writeSNEOpenAIError(w, http.StatusForbidden, "origin_not_allowed", "origin is not allowed", nil)
			return
		}
		if r.Method == http.MethodOptions || s.sneAccess == nil {
			next(w, r)
			return
		}
		provided, ok := parseBearerCredential(r.Header.Get("Authorization"))
		// The embedded Pantheon page receives a session-only HttpOnly cookie.
		// Hosted Nexus and API clients continue to use an explicit bearer. If an
		// Authorization header is present but malformed/invalid, never mask it
		// with a browser cookie.
		if !ok && strings.TrimSpace(r.Header.Get("Authorization")) == "" {
			if cookie, err := r.Cookie(sneLocalSessionCookie); err == nil && cookie.Value != "" {
				provided, ok = []byte(cookie.Value), true
			}
		}
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="sirsi-local"`)
			writeSNELocalCapabilityError(w, http.StatusUnauthorized, "local_capability_required", "a Pantheon local capability is required")
			return
		}
		if !s.sneAccess.matches(provided) {
			writeSNELocalCapabilityError(w, http.StatusForbidden, "local_capability_invalid", "the Pantheon local capability is invalid")
			return
		}
		next(w, r)
	}
}

func (s *Server) secureSNERoute(requireCapability bool, next http.HandlerFunc) http.HandlerFunc {
	if requireCapability {
		return requireLoopbackHost(s.requireSNECapability(next))
	}
	return requireLoopbackHost(next)
}

func parseBearerCredential(header string) ([]byte, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return nil, false
	}
	return []byte(parts[1]), true
}

func isLoopbackRequestHost(authority string) bool {
	authority = strings.TrimSpace(authority)
	if authority == "" {
		return false
	}
	if ip := net.ParseIP(authority); ip != nil {
		return ip.IsLoopback()
	}
	host := authority
	if parsedHost, port, err := net.SplitHostPort(authority); err == nil {
		portNumber, portErr := strconv.Atoi(port)
		if portErr != nil || portNumber < 1 || portNumber > 65535 {
			return false
		}
		host = parsedHost
	} else if strings.HasPrefix(authority, "[") && strings.HasSuffix(authority, "]") {
		host = strings.TrimSuffix(strings.TrimPrefix(authority, "["), "]")
	} else if strings.Contains(authority, ":") {
		// A colon that is neither a valid host:port separator nor a bracketed
		// IPv6 literal is an ambiguous authority and must fail closed.
		return false
	}
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
