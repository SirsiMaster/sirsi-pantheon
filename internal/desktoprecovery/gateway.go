// Package desktoprecovery provides Pantheon's narrow browser-to-RFB recovery
// bridge. It is deliberately not a general TCP proxy: every destination is a
// configured private node, every browser session is short lived, and the
// bridge trusts identity headers only from a loopback Tailscale Serve proxy.
package desktoprecovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/websocket"
)

const (
	cookieName        = "__Host-pantheon-recovery"
	defaultSessionTTL = 10 * time.Minute
	maxSessionTTL     = 30 * time.Minute
)

//go:embed novnc
var noVNC embed.FS

// Node is one explicitly approved Screen Sharing target. Address must be a
// literal RFC1918 or Tailscale IP on port 5900; hostnames and public addresses
// are rejected so browser input can never turn this service into a proxy.
type Node struct {
	ID      string   `json:"id"`
	Address string   `json:"address"`
	Origins []string `json:"origins"`
}

// Principal identifies the already-authenticated tailnet operator. It carries
// no secret and is used only to bind an ephemeral recovery session.
type Principal struct{ Login string }

// Authorizer is the Pantheon admission seam. Production uses
// TailnetHeaderAuthorizer behind a loopback-only Tailscale Serve proxy; tests
// inject a deterministic authorizer. There is intentionally no anonymous or
// static-token fallback.
type Authorizer interface {
	AuthorizeRecovery(context.Context, *http.Request, Node) (Principal, error)
}

// TailnetHeaderAuthorizer admits only explicitly configured Tailscale login
// names, and only when the immediate peer is loopback. Tailscale Serve strips
// spoofed identity headers before forwarding a tailnet request; binding the
// backend to loopback prevents a LAN/tailnet client from forging those headers.
type TailnetHeaderAuthorizer struct {
	AllowedLogins map[string]struct{}
}

func (a TailnetHeaderAuthorizer) AuthorizeRecovery(_ context.Context, r *http.Request, _ Node) (Principal, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	addr, parseErr := netip.ParseAddr(host)
	if err != nil || parseErr != nil || !addr.IsLoopback() {
		return Principal{}, errors.New("desktop recovery requires loopback Tailscale Serve proxy")
	}
	login := strings.TrimSpace(r.Header.Get("Tailscale-User-Login"))
	if login == "" {
		return Principal{}, errors.New("desktop recovery requires Tailscale user identity")
	}
	if _, ok := a.AllowedLogins[login]; !ok {
		return Principal{}, errors.New("desktop recovery operator is not allowlisted")
	}
	return Principal{Login: login}, nil
}

// Config supplies the sole source of node and operator authority. A caller
// cannot add destinations through a request or query string.
type Config struct {
	Nodes       []Node
	Authorizer  Authorizer
	SessionTTL  time.Duration
	Now         func() time.Time
	Rand        func([]byte) (int, error)
	DialContext func(context.Context, string, string) (net.Conn, error)
}

type session struct {
	nodeID    string
	principal string
	expiresAt time.Time
	active    bool
}

// Gateway serves an embedded, pinned noVNC client and a narrow authenticated
// WebSocket-to-RFB bridge. It does not execute desktop payloads or invoke a
// shell; it only carries the existing Screen Sharing protocol after admission.
type Gateway struct {
	nodes map[string]Node
	auth  Authorizer
	ttl   time.Duration
	now   func() time.Time
	rand  func([]byte) (int, error)
	dial  func(context.Context, string, string) (net.Conn, error)

	mu       sync.Mutex
	sessions map[[sha256.Size]byte]session
}

func New(cfg Config) (*Gateway, error) {
	if cfg.Authorizer == nil {
		return nil, errors.New("desktop recovery: authorizer is required")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Rand == nil {
		cfg.Rand = rand.Read
	}
	if cfg.DialContext == nil {
		d := &net.Dialer{Timeout: 5 * time.Second}
		cfg.DialContext = d.DialContext
	}
	if cfg.SessionTTL == 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.SessionTTL <= 0 || cfg.SessionTTL > maxSessionTTL {
		return nil, fmt.Errorf("desktop recovery: session TTL must be between 1ns and %s", maxSessionTTL)
	}
	nodes := make(map[string]Node, len(cfg.Nodes))
	for _, node := range cfg.Nodes {
		if err := validateNode(node); err != nil {
			return nil, err
		}
		if _, exists := nodes[node.ID]; exists {
			return nil, fmt.Errorf("desktop recovery: duplicate node %q", node.ID)
		}
		node.Origins = canonicalOrigins(node.Origins)
		nodes[node.ID] = node
	}
	if len(nodes) == 0 {
		return nil, errors.New("desktop recovery: at least one approved node is required")
	}
	return &Gateway{nodes: nodes, auth: cfg.Authorizer, ttl: cfg.SessionTTL, now: cfg.Now, rand: cfg.Rand, dial: cfg.DialContext, sessions: make(map[[sha256.Size]byte]session)}, nil
}

func validateNode(node Node) error {
	if node.ID == "" || strings.ContainsAny(node.ID, "/?&#") {
		return fmt.Errorf("desktop recovery: invalid node id %q", node.ID)
	}
	host, port, err := net.SplitHostPort(node.Address)
	if err != nil || port != "5900" {
		return fmt.Errorf("desktop recovery: node %q must use a literal private RFB address on port 5900", node.ID)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !isPrivateRFBAddress(ip) {
		return fmt.Errorf("desktop recovery: node %q address must be private or Tailscale", node.ID)
	}
	if len(node.Origins) == 0 {
		return fmt.Errorf("desktop recovery: node %q requires an HTTPS origin allowlist", node.ID)
	}
	for _, origin := range node.Origins {
		u, err := url.Parse(origin)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("desktop recovery: node %q has invalid HTTPS origin %q", node.ID, origin)
		}
	}
	return nil
}

func isPrivateRFBAddress(ip netip.Addr) bool {
	if ip.Is4() && ip.As4()[0] == 100 && ip.As4()[1]&0xc0 == 0x40 { // 100.64.0.0/10 Tailscale CGNAT range
		return true
	}
	return ip.IsPrivate()
}

func canonicalOrigins(origins []string) []string {
	set := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		set[origin] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for origin := range set {
		out = append(out, origin)
	}
	sort.Strings(out)
	return out
}

// Handler exposes only noVNC assets plus the bounded recovery API.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/recovery/novnc/", http.StripPrefix("/recovery/novnc/", http.FileServerFS(mustSub(noVNC, "novnc"))))
	mux.HandleFunc("/recovery/v1/nodes/", g.handleNode)
	return mux
}

func mustSub(root fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(root, dir)
	if err != nil {
		panic(err)
	}
	return sub
}

func (g *Gateway) handleNode(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(path.Clean(r.URL.Path), "/recovery/v1/nodes/")
	parts := strings.Split(rel, "/")
	if len(parts) != 2 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	node, ok := g.nodes[parts[0]]
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch parts[1] {
	case "sessions":
		g.admit(w, r, node)
	case "client":
		g.client(w, r, node)
	case "ws":
		g.websocket(node).ServeHTTP(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (g *Gateway) admit(w http.ResponseWriter, r *http.Request, node Node) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	if !originAllowed(r.Header.Get("Origin"), node) {
		http.Error(w, "recovery origin denied", http.StatusForbidden)
		return
	}
	principal, err := g.auth.AuthorizeRecovery(r.Context(), r, node)
	if err != nil {
		http.Error(w, "recovery admission denied", http.StatusUnauthorized)
		return
	}
	raw := make([]byte, 32)
	if n, err := g.rand(raw); err != nil || n != len(raw) {
		http.Error(w, "recovery session unavailable", http.StatusServiceUnavailable)
		return
	}
	now := g.now().UTC()
	expires := now.Add(g.ttl)
	hash := sha256.Sum256(raw)
	g.mu.Lock()
	g.gcLocked(now)
	g.sessions[hash] = session{nodeID: node.ID, principal: principal.Login, expiresAt: expires}
	g.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: hex.EncodeToString(raw), Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, Expires: expires})
	writeJSON(w, http.StatusCreated, map[string]string{"node": node.ID, "expires_at": expires.Format(time.RFC3339), "client_path": "/recovery/v1/nodes/" + node.ID + "/client"})
}

func (g *Gateway) client(w http.ResponseWriter, r *http.Request, node Node) {
	if r.Method != http.MethodGet || !g.validSession(r, node.ID, false) {
		http.Error(w, "active recovery admission required", http.StatusUnauthorized)
		return
	}
	q := url.Values{"autoconnect": {"true"}, "reconnect": {"false"}, "path": {"recovery/v1/nodes/" + node.ID + "/ws"}}
	http.Redirect(w, r, "/recovery/novnc/vnc.html?"+q.Encode(), http.StatusFound)
}

func (g *Gateway) websocket(node Node) websocket.Server {
	return websocket.Server{
		Handshake: func(_ *websocket.Config, r *http.Request) error {
			if !originAllowed(r.Header.Get("Origin"), node) {
				return errors.New("recovery origin denied")
			}
			if !g.validSession(r, node.ID, true) {
				return errors.New("active recovery admission required")
			}
			return nil
		},
		Handler: func(ws *websocket.Conn) { g.proxy(node, ws) },
	}
}

func (g *Gateway) proxy(node Node, ws *websocket.Conn) {
	defer ws.Close()
	request := ws.Request()
	hash, ok := sessionHash(request)
	if !ok {
		return
	}
	now := g.now().UTC()
	g.mu.Lock()
	g.gcLocked(now)
	s, exists := g.sessions[hash]
	if !exists || s.nodeID != node.ID || s.active || !now.Before(s.expiresAt) {
		g.mu.Unlock()
		return
	}
	s.active = true
	g.sessions[hash] = s
	g.mu.Unlock()
	defer func() {
		g.mu.Lock()
		delete(g.sessions, hash)
		g.mu.Unlock()
	}()

	ctx, cancel := context.WithDeadline(request.Context(), s.expiresAt)
	defer cancel()
	rfb, err := g.dial(ctx, "tcp", node.Address)
	if err != nil {
		return
	}
	defer rfb.Close()

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(rfb, ws); done <- struct{}{} }()
	go func() { _, _ = io.Copy(ws, rfb); done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func (g *Gateway) validSession(r *http.Request, nodeID string, requireIdle bool) bool {
	hash, ok := sessionHash(r)
	if !ok {
		return false
	}
	now := g.now().UTC()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.gcLocked(now)
	s, exists := g.sessions[hash]
	return exists && s.nodeID == nodeID && now.Before(s.expiresAt) && (!requireIdle || !s.active)
}

func sessionHash(r *http.Request) ([sha256.Size]byte, bool) {
	cookie, err := r.Cookie(cookieName)
	if err != nil || len(cookie.Value) != 64 {
		return [sha256.Size]byte{}, false
	}
	raw, err := hex.DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		return [sha256.Size]byte{}, false
	}
	return sha256.Sum256(raw), true
}

func (g *Gateway) gcLocked(now time.Time) {
	for key, s := range g.sessions {
		if !now.Before(s.expiresAt) {
			delete(g.sessions, key)
		}
	}
}

func originAllowed(origin string, node Node) bool {
	for _, allowed := range node.Origins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
